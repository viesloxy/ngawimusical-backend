-- Skema awal NgawiMusical Backend.
-- Semua tabel transaksi memakai waktu UTC dan nominal rupiah dalam BIGINT.

CREATE TABLE IF NOT EXISTS roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id INT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission VARCHAR(100) NOT NULL,
    PRIMARY KEY (role_id, permission)
);

INSERT INTO roles (name)
VALUES ('admin'), ('organizer'), ('gate_staff'), ('attendee')
ON CONFLICT (name) DO NOTHING;

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(120) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role_id INT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DELETED')),
    email_verified_at TIMESTAMPTZ NULL,
    auth_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    family_id UUID NOT NULL,
    parent_id BIGINT NULL REFERENCES refresh_tokens(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS identity_action_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    purpose VARCHAR(30) NOT NULL CHECK (purpose IN ('VERIFY_EMAIL', 'RESET_PASSWORD')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(40) NOT NULL UNIQUE,
    title VARCHAR(150) NOT NULL,
    venue VARCHAR(150) NOT NULL,
    venue_capacity INT NOT NULL CHECK (venue_capacity > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT', 'PUBLISHED', 'CANCELLED', 'COMPLETED', 'ARCHIVED')),
    sale_control VARCHAR(20) NOT NULL DEFAULT 'ENABLED' CHECK (sale_control IN ('ENABLED', 'PAUSED', 'CLOSED')),
    queue_epoch INT NOT NULL DEFAULT 1 CHECK (queue_epoch > 0),
    max_tickets_per_user INT NOT NULL DEFAULT 4 CHECK (max_tickets_per_user BETWEEN 1 AND 10),
    sale_open_at TIMESTAMPTZ NOT NULL,
    sale_close_at TIMESTAMPTZ NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    organizer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK (sale_open_at < sale_close_at AND sale_close_at < starts_at AND starts_at < ends_at)
);

CREATE INDEX IF NOT EXISTS events_public_cursor ON events(created_at DESC, id DESC) WHERE status = 'PUBLISHED';

CREATE TABLE IF NOT EXISTS event_staff (
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    staff_role VARCHAR(30) NOT NULL CHECK (staff_role IN ('GATE_STAFF', 'ORGANIZER_STAFF')),
    gate_id VARCHAR(50) NOT NULL DEFAULT 'MAIN_GATE',
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'REVOKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (event_id, user_id)
);

CREATE TABLE IF NOT EXISTS ticket_tiers (
    id BIGSERIAL PRIMARY KEY,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    code VARCHAR(40) NOT NULL,
    name VARCHAR(100) NOT NULL,
    price_amount BIGINT NOT NULL CHECK (price_amount > 0),
    fee_amount BIGINT NOT NULL DEFAULT 0 CHECK (fee_amount >= 0),
    quota INT NOT NULL CHECK (quota >= 0),
    held_count INT NOT NULL DEFAULT 0,
    sold_count INT NOT NULL DEFAULT 0,
    retired_count INT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CHECK (held_count >= 0 AND sold_count >= 0 AND retired_count >= 0),
    CHECK (held_count + sold_count + retired_count <= quota),
    UNIQUE (event_id, code),
    UNIQUE (event_id, id)
);

CREATE TABLE IF NOT EXISTS user_event_limits (
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    held INT NOT NULL DEFAULT 0,
    purchased_total INT NOT NULL DEFAULT 0,
    frozen_max INT NOT NULL CHECK (frozen_max > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (event_id, user_id),
    CHECK (held >= 0 AND purchased_total >= 0),
    CHECK (held + purchased_total <= frozen_max)
);

CREATE TABLE IF NOT EXISTS bookings (
    id BIGSERIAL PRIMARY KEY,
    booking_code VARCHAR(40) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    tier_id BIGINT NOT NULL,
    quantity INT NOT NULL CHECK (quantity BETWEEN 1 AND 4),
    status VARCHAR(25) NOT NULL DEFAULT 'HELD' CHECK (status IN ('HELD', 'PAYMENT_PENDING', 'CONFIRMED', 'CANCELLED', 'EXPIRED', 'REFUND_PENDING', 'REFUNDED')),
    unit_price_amount BIGINT NOT NULL CHECK (unit_price_amount > 0),
    unit_fee_amount BIGINT NOT NULL DEFAULT 0 CHECK (unit_fee_amount >= 0),
    total_amount BIGINT NOT NULL CHECK (total_amount = quantity * (unit_price_amount + unit_fee_amount)),
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR' CHECK (currency = 'IDR'),
    expires_at TIMESTAMPTZ NOT NULL,
    release_at TIMESTAMPTZ NOT NULL CHECK (release_at > expires_at),
    first_confirmed_at TIMESTAMPTZ NULL,
    swap_count INT NOT NULL DEFAULT 0 CHECK (swap_count BETWEEN 0 AND 2),
    version INT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (event_id, tier_id) REFERENCES ticket_tiers(event_id, id) ON DELETE RESTRICT,
    UNIQUE (event_id, tier_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS bookings_one_active_per_user_event
ON bookings(event_id, user_id)
WHERE status IN ('HELD', 'PAYMENT_PENDING');

CREATE INDEX IF NOT EXISTS bookings_due_release ON bookings(release_at, id)
WHERE status IN ('HELD', 'PAYMENT_PENDING');

CREATE INDEX IF NOT EXISTS bookings_owner_cursor ON bookings(user_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS booking_changes (
    id BIGSERIAL PRIMARY KEY,
    operation_id VARCHAR(128) NOT NULL UNIQUE,
    booking_id BIGINT NOT NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    old_tier_id BIGINT NOT NULL,
    new_tier_id BIGINT NOT NULL,
    old_total_amount BIGINT NOT NULL,
    new_total_amount BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS admission_redemptions (
    jti VARCHAR(100) PRIMARY KEY,
    booking_id BIGINT NOT NULL UNIQUE REFERENCES bookings(id) ON DELETE CASCADE,
    event_id BIGINT NOT NULL REFERENCES events(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    queue_epoch INT NOT NULL,
    redeemed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS idempotency_records (
    user_id BIGINT NOT NULL,
    operation_scope VARCHAR(100) NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    response_status INT NULL,
    response_body JSONB NULL,
    resource_id VARCHAR(64) NULL,
    completed_at TIMESTAMPTZ NULL,
    retention_until TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (user_id, operation_scope, idempotency_key)
);

CREATE TABLE IF NOT EXISTS payment_attempts (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL UNIQUE REFERENCES bookings(id) ON DELETE RESTRICT,
    merchant_order_id VARCHAR(80) NOT NULL UNIQUE,
    provider VARCHAR(40) NOT NULL DEFAULT 'SIMULATOR',
    provider_transaction_id VARCHAR(100) NULL UNIQUE,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'IDR' CHECK (currency = 'IDR'),
    status VARCHAR(25) NOT NULL DEFAULT 'CREATE_PENDING' CHECK (status IN ('CREATE_PENDING', 'PENDING', 'UNKNOWN', 'SUCCEEDED', 'FAILED', 'REFUND_PENDING', 'REFUNDED')),
    provider_paid_at TIMESTAMPTZ NULL,
    observed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS payment_inbox (
    id BIGSERIAL PRIMARY KEY,
    dedupe_key VARCHAR(160) NOT NULL UNIQUE,
    provider VARCHAR(40) NOT NULL,
    payload_hash CHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    processing_state VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (processing_state IN ('PENDING', 'PROCESSING', 'DONE', 'FAILED')),
    lease_until TIMESTAMPTZ NULL,
    retry_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    last_error TEXT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX IF NOT EXISTS payment_inbox_due ON payment_inbox(retry_at, id) WHERE processing_state <> 'DONE';

CREATE TABLE IF NOT EXISTS refunds (
    id BIGSERIAL PRIMARY KEY,
    case_key VARCHAR(120) NOT NULL UNIQUE,
    payment_attempt_id BIGINT NOT NULL REFERENCES payment_attempts(id) ON DELETE RESTRICT,
    booking_id BIGINT NOT NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    reason VARCHAR(40) NOT NULL CHECK (reason IN ('EVENT_CANCELLED', 'UNALLOCATED_PAYMENT')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'UNKNOWN', 'SUCCEEDED', 'FAILED')),
    provider_refund_id VARCHAR(100) NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS tickets (
    id BIGSERIAL PRIMARY KEY,
    booking_id BIGINT NOT NULL,
    ordinal INT NOT NULL CHECK (ordinal > 0),
    event_id BIGINT NOT NULL,
    tier_id BIGINT NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    token_ciphertext TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'VALID' CHECK (status IN ('VALID', 'CHECKED_IN', 'VOID')),
    checked_in_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (booking_id, ordinal),
    FOREIGN KEY (event_id, tier_id, booking_id) REFERENCES bookings(event_id, tier_id, id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS check_ins (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL UNIQUE REFERENCES tickets(id) ON DELETE RESTRICT,
    staff_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    gate_id VARCHAR(50) NOT NULL,
    operation_id VARCHAR(128) NOT NULL UNIQUE,
    scanned_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS inventory_movements (
    id BIGSERIAL PRIMARY KEY,
    operation_id VARCHAR(128) NOT NULL,
    tier_id BIGINT NOT NULL REFERENCES ticket_tiers(id) ON DELETE RESTRICT,
    booking_id BIGINT NOT NULL REFERENCES bookings(id) ON DELETE RESTRICT,
    movement_type VARCHAR(30) NOT NULL CHECK (movement_type IN ('HOLD', 'RELEASE_CANCEL', 'RELEASE_EXPIRE', 'SWAP_OUT', 'SWAP_IN', 'CONFIRM_SALE', 'RETIRE_REFUND')),
    delta_held INT NOT NULL DEFAULT 0,
    delta_sold INT NOT NULL DEFAULT 0,
    delta_retired INT NOT NULL DEFAULT 0,
    reason VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (operation_id, tier_id, movement_type)
);

CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    aggregate_id VARCHAR(64) NOT NULL,
    aggregate_version INT NOT NULL DEFAULT 1,
    event_type VARCHAR(60) NOT NULL,
    schema_version INT NOT NULL DEFAULT 1,
    payload JSONB NOT NULL,
    lease_owner VARCHAR(64) NULL,
    lease_until TIMESTAMPTZ NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    published_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX IF NOT EXISTS outbox_due_idx ON outbox(next_attempt_at, created_at) WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS consumer_receipts (
    consumer VARCHAR(60) NOT NULL,
    event_id UUID NOT NULL REFERENCES outbox(id) ON DELETE CASCADE,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (consumer, event_id)
);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL REFERENCES outbox(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel VARCHAR(20) NOT NULL DEFAULT 'EMAIL',
    recipient VARCHAR(150) NOT NULL,
    subject VARCHAR(200) NOT NULL,
    body TEXT NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING', 'SENT', 'UNKNOWN', 'FAILED_PERMANENT')),
    provider_key VARCHAR(120) NOT NULL UNIQUE,
    provider_message_id VARCHAR(120) NULL,
    lease_until TIMESTAMPTZ NULL,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (event_id, channel, recipient)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id BIGINT NULL,
    action VARCHAR(80) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(64) NOT NULL,
    request_id VARCHAR(64) NOT NULL,
    changes JSONB NULL,
    reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS operational_exceptions (
    id BIGSERIAL PRIMARY KEY,
    case_key VARCHAR(120) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL,
    resource_id VARCHAR(64) NOT NULL,
    owner VARCHAR(60) NOT NULL DEFAULT 'UNASSIGNED',
    state VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN', 'INVESTIGATING', 'RESOLVED')),
    evidence JSONB NOT NULL,
    next_action_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    resolved_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
