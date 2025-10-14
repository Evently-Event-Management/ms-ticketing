CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

DROP TABLE IF EXISTS tickets;
DROP TABLE IF EXISTS orders;

CREATE TABLE orders (
    order_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    event_id UUID,
    organization_id UUID,
    session_id UUID NOT NULL,
    status   TEXT NOT NULL,
    subtotal NUMERIC(10,2) NOT NULL,
    discount_id UUID,
    discount_code TEXT,
    discount_amount NUMERIC(10,2) DEFAULT 0,
    price    NUMERIC(10,2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    payment_intent_id TEXT
);

CREATE TABLE tickets (
    ticket_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id  UUID NOT NULL REFERENCES orders(order_id) ON DELETE CASCADE,
    seat_id   UUID NOT NULL,
    seat_label TEXT NOT NULL,
    colour     TEXT NOT NULL,
    tier_id    UUID NOT NULL,
    tier_name  TEXT NOT NULL,
    qr_code    BYTEA,
    price_at_purchase NUMERIC(10,2) NOT NULL,
    issued_at TIMESTAMP NOT NULL DEFAULT NOW(),
    checked_in BOOLEAN NOT NULL DEFAULT FALSE,
    checked_in_time TIMESTAMP
);