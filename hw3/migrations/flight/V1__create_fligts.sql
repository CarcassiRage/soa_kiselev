CREATE TABLE airlines (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code TEXT NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9]{2}$'),
    name TEXT NOT NULL CHECK (btrim(name) <> '')
);

CREATE TABLE flights (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    airline_id BIGINT NOT NULL REFERENCES airlines(id),
    flight_number TEXT NOT NULL CHECK (btrim(flight_number) <> ''),
    origin TEXT NOT NULL CHECK (origin ~ '^[A-Z]{3}$'),
    destination TEXT NOT NULL CHECK (destination ~ '^[A-Z]{3}$'),
    departure_at TIMESTAMPTZ NOT NULL,
    arrival_at TIMESTAMPTZ NOT NULL,
    total_seats INTEGER NOT NULL CHECK (total_seats BETWEEN 1 AND 10000),
    available_seats INTEGER NOT NULL CHECK (available_seats BETWEEN 0 AND total_seats),
    price BIGINT NOT NULL CHECK (price BETWEEN 1 AND 1000000000),
    status TEXT NOT NULL CHECK (status IN ('SCHEDULED', 'DEPARTED', 'CANCELLED', 'COMPLETED')),
    CHECK (origin <> destination),
    CHECK (arrival_at > departure_at)
);

CREATE UNIQUE INDEX flights_number_date ON flights (flight_number, ((departure_at AT TIME ZONE 'UTC')::date));
CREATE INDEX flights_search ON flights (origin, destination, departure_at) WHERE status = 'SCHEDULED';

CREATE TABLE seat_reservations (
    booking_id UUID PRIMARY KEY,
    flight_id BIGINT NOT NULL REFERENCES flights(id),
    seat_count INTEGER NOT NULL CHECK (seat_count BETWEEN 1 AND 10000),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'RELEASED', 'EXPIRED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX reservations_flight ON seat_reservations (flight_id);
