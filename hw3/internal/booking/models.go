package booking

import (
	"strings"
	"time"

	pb "hw3/gen/flight/v1"

	"github.com/jackc/pgx/v5"
)

type Booking struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	FlightID       int32     `json:"flight_id"`
	PassengerName  string    `json:"passenger_name"`
	PassengerEmail string    `json:"passenger_email"`
	SeatCount      int32     `json:"seat_count"`
	TotalPrice     int64     `json:"total_price"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type Flight struct {
	ID             int32     `json:"id"`
	FlightNumber   string    `json:"flight_number"`
	AirlineCode    string    `json:"airline_code"`
	AirlineName    string    `json:"airline_name"`
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	DepartureAt    time.Time `json:"departure_at"`
	ArrivalAt      time.Time `json:"arrival_at"`
	TotalSeats     int32     `json:"total_seats"`
	AvailableSeats int32     `json:"available_seats"`
	Price          int32     `json:"price"`
	Status         string    `json:"status"`
}

type createRequest struct {
	UserID         string `json:"user_id"`
	FlightID       int32  `json:"flight_id"`
	PassengerName  string `json:"passenger_name"`
	PassengerEmail string `json:"passenger_email"`
	SeatCount      int32  `json:"seat_count"`
}

func flightDTO(f *pb.Flight) Flight {
	return Flight{
		ID:             f.Id,
		FlightNumber:   f.FlightNumber,
		AirlineCode:    f.AirlineCode,
		AirlineName:    f.AirlineName,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureAt:    f.DepartureAt.AsTime(),
		ArrivalAt:      f.ArrivalAt.AsTime(),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		Price:          f.Price,
		Status:         strings.TrimPrefix(f.Status.String(), "FLIGHT_STATUS_"),
	}
}

const bookingSelect = `SELECT id::text, user_id, flight_id, passenger_name, passenger_email, seat_count,
							 total_price, status, created_at FROM bookings`

func scanBooking(row pgx.Row) (Booking, error) {
	var b Booking
	err := row.Scan(&b.ID, &b.UserID, &b.FlightID, &b.PassengerName, &b.PassengerEmail, &b.SeatCount,
		&b.TotalPrice, &b.Status, &b.CreatedAt)
	return b, err
}
