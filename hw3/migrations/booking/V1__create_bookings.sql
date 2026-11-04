CREATE TABLE bookings (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL CHECK (btrim(user_id) <> ''),
    flight_id BIGINT NOT NULL CHECK (flight_id > 0),
    passenger_name TEXT NOT NULL CHECK (btrim(passenger_name) <> ''),
    passenger_email TEXT NOT NULL CHECK (btrim(passenger_email) <> ''),
    seat_count INTEGER NOT NULL CHECK (seat_count BETWEEN 1 AND 10000),
    total_price BIGINT NOT NULL CHECK (total_price > 0),
    status TEXT NOT NULL CHECK (status IN ('CONFIRMED', 'CANCELLED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX bookings_user ON bookings (user_id, created_at, id);
