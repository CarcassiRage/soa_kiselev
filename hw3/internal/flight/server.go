package flight

import (
	"context"
	"errors"
	"regexp"
	"time"

	pb "hw3/gen/flight/v1"
	"hw3/internal/platform"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	pb.UnimplementedFlightServiceServer
	db *pgxpool.Pool
}

func NewServer(db *pgxpool.Pool) *Server {
	return &Server{db: db}
}

var iata = regexp.MustCompile(`^[A-Z]{3}$`)

const flightSelect = `SELECT f.id, f.flight_number, a.code, a.name, f.origin, f.destination, f.departure_at, 
					  f.arrival_at, f.total_seats, f.available_seats, f.price, f.status
					  FROM flights f JOIN airlines a ON a.id = f.airline_id`

const reservationSelect = `SELECT booking_id::text, flight_id, seat_count, status FROM seat_reservations WHERE booking_id = $1`

func scanFlight(row pgx.Row) (*pb.Flight, error) {
	f := &pb.Flight{}
	var departure, arrival time.Time
	var state string
	err := row.Scan(&f.Id, &f.FlightNumber, &f.AirlineCode, &f.AirlineName, &f.Origin, &f.Destination, &departure,
		&arrival, &f.TotalSeats, &f.AvailableSeats, &f.Price, &state)
	if err != nil {
		return nil, err
	}
	f.DepartureAt = timestamppb.New(departure)
	f.ArrivalAt = timestamppb.New(arrival)
	f.Status = pb.FlightStatus(pb.FlightStatus_value["FLIGHT_STATUS_"+state])
	return f, nil
}

func scanReservation(row pgx.Row) (*pb.SeatReservation, error) {
	r := &pb.SeatReservation{}
	var state string
	err := row.Scan(&r.BookingId, &r.FlightId, &r.SeatCount, &state)
	if err != nil {
		return nil, err
	}
	r.Status = pb.ReservationStatus(pb.ReservationStatus_value["RESERVATION_STATUS_"+state])
	return r, nil
}

func storageError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return status.FromContextError(ctx.Err()).Err()
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return status.Error(codes.NotFound, "not found")
	}
	return status.Error(codes.Internal, "database operation failed")
}

func (s *Server) SearchFlights(ctx context.Context, req *pb.SearchFlightsRequest) (*pb.SearchFlightsResponse, error) {
	if !iata.MatchString(req.Origin) || !iata.MatchString(req.Destination) || req.Origin == req.Destination {
		return nil, status.Error(codes.InvalidArgument, "origin and destination must be different uppercase IATA codes")
	}

	query := flightSelect + ` WHERE f.origin = $1 AND f.destination = $2 AND f.status = 'SCHEDULED'
							  ORDER BY f.departure_at, f.id`
	rows, err := s.db.Query(ctx, query, req.Origin, req.Destination)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer rows.Close()

	result := &pb.SearchFlightsResponse{Flights: []*pb.Flight{}}
	for rows.Next() {
		f, err := scanFlight(rows)
		if err != nil {
			return nil, storageError(ctx, err)
		}
		result.Flights = append(result.Flights, f)
	}
	if err := rows.Err(); err != nil {
		return nil, storageError(ctx, err)
	}
	return result, nil
}

func (s *Server) GetFlight(ctx context.Context, req *pb.GetFlightRequest) (*pb.Flight, error) {
	if req.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id must be positive")
	}
	f, err := scanFlight(s.db.QueryRow(ctx, flightSelect+` WHERE f.id = $1`, req.Id))
	if err != nil {
		return nil, storageError(ctx, err)
	}
	return f, nil
}

func (s *Server) ReserveSeats(ctx context.Context, req *pb.ReserveSeatsRequest) (*pb.SeatReservation, error) {
	id, err := uuid.Parse(req.BookingId)
	if err != nil || id == uuid.Nil || req.FlightId <= 0 || req.SeatCount <= 0 || req.SeatCount > 10000 {
		return nil, status.Error(codes.InvalidArgument, "valid booking UUID, positive flight_id and seat_count (up to 10000) required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer platform.Rollback(tx)

	var available int32
	var state string
	var departure time.Time

	err = tx.QueryRow(ctx, `SELECT available_seats, status, departure_at FROM flights WHERE id = $1 FOR UPDATE`,
		req.FlightId).Scan(&available, &state, &departure)
	if err != nil {
		return nil, storageError(ctx, err)
	}

	existing, err := scanReservation(tx.QueryRow(ctx, reservationSelect+` FOR UPDATE`, id.String()))
	if err == nil {
		if existing.FlightId != req.FlightId || existing.SeatCount != req.SeatCount {
			return nil, status.Error(codes.AlreadyExists, "booking_id already has another reservation")
		}
		if existing.Status != pb.ReservationStatus_RESERVATION_STATUS_ACTIVE {
			return nil, status.Error(codes.FailedPrecondition, "reservation is no longer active")
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, storageError(ctx, err)
	}

	if state != "SCHEDULED" || !departure.After(time.Now()) {
		return nil, status.Error(codes.FailedPrecondition, "flight is not available for booking")
	}
	if available < req.SeatCount {
		return nil, status.Error(codes.ResourceExhausted, "not enough available seats")
	}

	_, err = tx.Exec(ctx, `UPDATE flights SET available_seats = available_seats - $2 WHERE id = $1`,
		req.FlightId, req.SeatCount)
	if err != nil {
		return nil, storageError(ctx, err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO seat_reservations (booking_id, flight_id, seat_count, status)
						  VALUES ($1, $2, $3, 'ACTIVE')`, id.String(), req.FlightId, req.SeatCount)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, status.Error(codes.AlreadyExists, "booking_id already has another reservation")
		}
		return nil, storageError(ctx, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, storageError(ctx, err)
	}

	return &pb.SeatReservation{
		BookingId: id.String(),
		FlightId:  req.FlightId,
		SeatCount: req.SeatCount,
		Status:    pb.ReservationStatus_RESERVATION_STATUS_ACTIVE,
	}, nil
}

func (s *Server) ReleaseReservation(ctx context.Context, req *pb.ReleaseReservationRequest) (*pb.SeatReservation, error) {
	id, err := uuid.Parse(req.BookingId)
	if err != nil || id == uuid.Nil {
		return nil, status.Error(codes.InvalidArgument, "valid booking UUID required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer platform.Rollback(tx)

	var flightID int32
	err = tx.QueryRow(ctx, `SELECT f.id FROM flights f JOIN seat_reservations r ON r.flight_id = f.id
						   WHERE r.booking_id = $1 FOR UPDATE OF f`, id.String()).Scan(&flightID)
	if err != nil {
		return nil, storageError(ctx, err)
	}

	r, err := scanReservation(tx.QueryRow(ctx, reservationSelect+` FOR UPDATE`, id.String()))
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if r.Status != pb.ReservationStatus_RESERVATION_STATUS_ACTIVE {
		return r, nil
	}

	_, err = tx.Exec(ctx, `UPDATE flights SET available_seats = available_seats + $2 WHERE id = $1`,
		flightID, r.SeatCount)
	if err != nil {
		return nil, storageError(ctx, err)
	}

	_, err = tx.Exec(ctx, `UPDATE seat_reservations SET status = 'RELEASED' WHERE booking_id = $1`, id.String())
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, storageError(ctx, err)
	}

	r.Status = pb.ReservationStatus_RESERVATION_STATUS_RELEASED
	return r, nil
}
