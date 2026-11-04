package booking

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	pb "hw3/gen/flight/v1"
	"hw3/internal/platform"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	db     *pgxpool.Pool
	flight pb.FlightServiceClient
}

func NewHandler(db *pgxpool.Pool, flight pb.FlightServiceClient) *gin.Engine {
	h := &Handler{db: db, flight: flight}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/healthz", h.health)
	r.GET("/flights", h.searchFlights)
	r.GET("/flights/:id", h.getFlight)
	r.POST("/bookings", h.createBooking)
	r.GET("/bookings/:id", h.getBooking)
	r.GET("/bookings", h.listBookings)
	r.POST("/bookings/:id/cancel", h.cancelBooking)
	return r
}

func (h *Handler) health(c *gin.Context) {
	if err := h.db.Ping(c.Request.Context()); err != nil {
		fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) searchFlights(c *gin.Context) {
	req := &pb.SearchFlightsRequest{
		Origin:      c.Query("origin"),
		Destination: c.Query("destination"),
	}
	result, err := h.flight.SearchFlights(c.Request.Context(), req)
	if err != nil {
		rpcError(c, err)
		return
	}

	flights := make([]Flight, 0, len(result.Flights))
	for _, f := range result.Flights {
		flights = append(flights, flightDTO(f))
	}
	c.JSON(http.StatusOK, flights)
}

func (h *Handler) getFlight(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 32)
	if err != nil || id <= 0 {
		fail(c, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	f, err := h.flight.GetFlight(c.Request.Context(), &pb.GetFlightRequest{Id: int32(id)})
	if err != nil {
		rpcError(c, err)
		return
	}
	c.JSON(http.StatusOK, flightDTO(f))
}

func (h *Handler) createBooking(c *gin.Context) {
	var req createRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	req.PassengerName = strings.TrimSpace(req.PassengerName)
	req.PassengerEmail = strings.TrimSpace(req.PassengerEmail)

	address, err := mail.ParseAddress(req.PassengerEmail)
	if req.UserID == "" || req.PassengerName == "" || err != nil || address.Address != req.PassengerEmail ||
		req.FlightID <= 0 || req.SeatCount <= 0 || req.SeatCount > 10000 {
		fail(c, http.StatusBadRequest, "user_id, passenger_name, valid passenger_email, positive flight_id and seat_count (up to 10000) required")
		return
	}

	f, err := h.flight.GetFlight(c.Request.Context(), &pb.GetFlightRequest{Id: req.FlightID})
	if err != nil {
		rpcError(c, err)
		return
	}

	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		dbError(c, err)
		return
	}
	defer platform.Rollback(tx)

	id := uuid.NewString()
	b := Booking{
		ID:             id,
		UserID:         req.UserID,
		FlightID:       req.FlightID,
		PassengerName:  req.PassengerName,
		PassengerEmail: req.PassengerEmail,
		SeatCount:      req.SeatCount,
		TotalPrice:     int64(req.SeatCount) * int64(f.Price),
		Status:         "CONFIRMED",
	}

	query := `INSERT INTO bookings (id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, 'CONFIRMED') RETURNING created_at`
	err = tx.QueryRow(c.Request.Context(), query,
		b.ID, b.UserID, b.FlightID, b.PassengerName, b.PassengerEmail, b.SeatCount, b.TotalPrice).Scan(&b.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail(c, http.StatusConflict, "booking ID already exists")
			return
		}
		dbError(c, err)
		return
	}

	_, err = h.flight.ReserveSeats(c.Request.Context(), &pb.ReserveSeatsRequest{
		FlightId:  req.FlightID,
		SeatCount: req.SeatCount,
		BookingId: id,
	})
	if err != nil {
		platform.Rollback(tx)
		switch status.Code(err) {
		case codes.Canceled, codes.DeadlineExceeded, codes.Unavailable, codes.Unknown, codes.Internal:
			if releaseErr := h.compensate(id); releaseErr != nil {
				rpcError(c, releaseErr)
				return
			}
		}
		rpcError(c, err)
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		var pgErr *pgconn.PgError
		if errors.Is(err, pgx.ErrTxCommitRollback) || errors.As(err, &pgErr) {
			if releaseErr := h.compensate(id); releaseErr != nil {
				rpcError(c, releaseErr)
				return
			}
		}
		dbError(c, err)
		return
	}

	c.Header("Location", "/bookings/"+id)
	c.JSON(http.StatusCreated, b)
}

func (h *Handler) compensate(id string) error {
	_, err := h.flight.ReleaseReservation(context.Background(), &pb.ReleaseReservationRequest{BookingId: id})
	if status.Code(err) == codes.NotFound {
		return nil
	}
	return err
}

func (h *Handler) getBooking(c *gin.Context) {
	id, ok := bookingID(c)
	if !ok {
		return
	}
	b, err := scanBooking(h.db.QueryRow(c.Request.Context(), bookingSelect+` WHERE id = $1`, id))
	if err != nil {
		dbError(c, err)
		return
	}
	c.JSON(http.StatusOK, b)
}

func (h *Handler) listBookings(c *gin.Context) {
	userID := strings.TrimSpace(c.Query("user_id"))
	if userID == "" {
		fail(c, http.StatusBadRequest, "user_id is required")
		return
	}

	rows, err := h.db.Query(c.Request.Context(), bookingSelect+` WHERE user_id = $1 ORDER BY created_at, id`, userID)
	if err != nil {
		dbError(c, err)
		return
	}
	defer rows.Close()

	bookings := []Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			dbError(c, err)
			return
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		dbError(c, err)
		return
	}
	c.JSON(http.StatusOK, bookings)
}

func (h *Handler) cancelBooking(c *gin.Context) {
	id, ok := bookingID(c)
	if !ok {
		return
	}

	tx, err := h.db.Begin(c.Request.Context())
	if err != nil {
		dbError(c, err)
		return
	}
	defer platform.Rollback(tx)

	b, err := scanBooking(tx.QueryRow(c.Request.Context(), bookingSelect+` WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		dbError(c, err)
		return
	}
	if b.Status != "CONFIRMED" {
		fail(c, http.StatusConflict, "booking is already cancelled")
		return
	}

	_, err = tx.Exec(c.Request.Context(), `UPDATE bookings SET status = 'CANCELLED' WHERE id = $1`, id)
	if err != nil {
		dbError(c, err)
		return
	}

	_, err = h.flight.ReleaseReservation(c.Request.Context(), &pb.ReleaseReservationRequest{BookingId: id})
	if err != nil {
		rpcError(c, err)
		return
	}
	if err := tx.Commit(c.Request.Context()); err != nil {
		dbError(c, err)
		return
	}

	b.Status = "CANCELLED"
	c.JSON(http.StatusOK, b)
}

func fail(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, gin.H{"reason": message})
}

func rpcError(c *gin.Context, err error) {
	s := status.Convert(err)
	code := http.StatusInternalServerError
	message := s.Message()

	switch s.Code() {
	case codes.InvalidArgument:
		code = http.StatusBadRequest
	case codes.NotFound:
		code = http.StatusNotFound
	case codes.ResourceExhausted, codes.AlreadyExists, codes.FailedPrecondition:
		code = http.StatusConflict
	case codes.Unavailable:
		code = http.StatusServiceUnavailable
		message = "flight service unavailable"
	case codes.DeadlineExceeded:
		code = http.StatusGatewayTimeout
		message = "flight service timed out"
	case codes.Canceled:
		code = http.StatusRequestTimeout
		message = "request cancelled"
	default:
		message = "flight service error"
	}

	fail(c, code, message)
}

func dbError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, http.StatusNotFound, "booking not found")
		return
	}
	fail(c, http.StatusInternalServerError, "database operation failed")
}

func bookingID(c *gin.Context) (string, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		fail(c, http.StatusBadRequest, "id must be a valid UUID")
		return "", false
	}
	return id.String(), true
}
