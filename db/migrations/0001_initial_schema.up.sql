-- LibraNexus Database Schema
-- PostgreSQL 16+ with event sourcing and partitioning

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm"; -- for fuzzy text search
CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- for encryption

-- ============================================================================
-- EVENT STORE (Core event sourcing tables)
-- ============================================================================

-- Events table with partitioning by aggregate_id hash
CREATE TABLE events (
    id BIGSERIAL,
    aggregate_id UUID NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    event_data JSONB NOT NULL,
    metadata JSONB,
    version INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (aggregate_id, version)
) PARTITION BY HASH (aggregate_id);

-- Create 8 partitions for load distribution
CREATE TABLE events_p0 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE events_p1 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 1);
CREATE TABLE events_p2 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 2);
CREATE TABLE events_p3 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 3);
CREATE TABLE events_p4 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 4);
CREATE TABLE events_p5 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 5);
CREATE TABLE events_p6 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 6);
CREATE TABLE events_p7 PARTITION OF events FOR VALUES WITH (MODULUS 8, REMAINDER 7);

-- Indexes for event streaming and querying
CREATE INDEX idx_events_id ON events (id);
CREATE INDEX idx_events_created_at ON events (created_at);
CREATE INDEX idx_events_type ON events (event_type, created_at);

-- Snapshots for performance optimization
CREATE TABLE snapshots (
    aggregate_id UUID PRIMARY KEY,
    aggregate_type VARCHAR(50) NOT NULL,
    version INT NOT NULL,
    state JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_snapshots_type ON snapshots (aggregate_type);

-- ============================================================================
-- CATALOG DOMAIN (Read model - materialized views)
-- ============================================================================

CREATE TABLE items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    isbn VARCHAR(13) NOT NULL,
    title VARCHAR(500) NOT NULL,
    author VARCHAR(500) NOT NULL,
    publisher VARCHAR(200),
    published_year INT,
    total_copies INT NOT NULL DEFAULT 1 CHECK (total_copies >= 0),
    available INT NOT NULL DEFAULT 1 CHECK (available >= 0),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired', 'lost', 'damaged')),
    version INT NOT NULL DEFAULT 1,
    marc_record JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_available_copies CHECK (available <= total_copies)
);

-- Indexes for search and availability queries
CREATE INDEX idx_items_isbn ON items (isbn);
CREATE INDEX idx_items_title_gin ON items USING gin (to_tsvector('english', title));
CREATE INDEX idx_items_author_gin ON items USING gin (to_tsvector('english', author));
CREATE INDEX idx_items_title_trgm ON items USING gin (title gin_trgm_ops);
CREATE INDEX idx_items_author_trgm ON items USING gin (author gin_trgm_ops);
CREATE INDEX idx_items_status ON items (status) WHERE status = 'active';
CREATE INDEX idx_items_available ON items (available) WHERE available > 0;

-- Inventory corrections audit trail
CREATE TABLE inventory_corrections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id UUID NOT NULL REFERENCES items(id),
    previous_copies INT NOT NULL,
    new_copies INT NOT NULL,
    reason TEXT NOT NULL,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_inventory_corrections_item ON inventory_corrections (item_id, created_at DESC);

-- ============================================================================
-- CIRCULATION DOMAIN
-- ============================================================================

CREATE TABLE checkouts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    member_id UUID NOT NULL,
    item_id UUID NOT NULL REFERENCES items(id),
    checkout_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    due_date TIMESTAMPTZ NOT NULL,
    return_date TIMESTAMPTZ,
    renewal_count INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'returned', 'overdue', 'lost')),
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for circulation queries
CREATE INDEX idx_checkouts_member ON checkouts (member_id, status);
CREATE INDEX idx_checkouts_item ON checkouts (item_id, return_date);
CREATE INDEX idx_checkouts_due_date ON checkouts (due_date) WHERE status = 'active';
CREATE INDEX idx_checkouts_overdue ON checkouts (due_date, status) WHERE status = 'active' AND due_date < NOW();

-- Reservations for waitlist management
CREATE TABLE reservations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    member_id UUID NOT NULL,
    item_id UUID NOT NULL REFERENCES items(id),
    reserved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'fulfilled', 'expired', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reservations_member ON reservations (member_id, status);
CREATE INDEX idx_reservations_item ON reservations (item_id, status, reserved_at);

-- ============================================================================
-- MEMBERSHIP DOMAIN
-- ============================================================================

CREATE TABLE members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(200) NOT NULL,
    membership_tier VARCHAR(20) NOT NULL DEFAULT 'basic' CHECK (membership_tier IN ('basic', 'premium', 'librarian')),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'expired')),
    version INT NOT NULL DEFAULT 1,
    fine_balance DECIMAL(10, 2) NOT NULL DEFAULT 0.00 CHECK (fine_balance >= 0),
    max_checkouts INT NOT NULL DEFAULT 5,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

CREATE INDEX idx_members_email ON members (email);
CREATE INDEX idx_members_status ON members (status);
CREATE INDEX idx_members_expires_at ON members (expires_at) WHERE status = 'active';

-- Credentials table (separate for security)
CREATE TABLE credentials (
    member_id UUID PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mfa_secret TEXT,
    failed_attempts INT NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fine transactions ledger
CREATE TABLE fine_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    member_id UUID NOT NULL REFERENCES members(id),
    amount DECIMAL(10, 2) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('fine', 'payment', 'waiver', 'adjustment')),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_fine_transactions_member ON fine_transactions (member_id, created_at DESC);

-- ============================================================================
-- AUDIT LOG
-- ============================================================================

CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    actor_id UUID,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    changes JSONB,
    ip_address INET,
    user_agent TEXT,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    success BOOLEAN NOT NULL DEFAULT TRUE
) PARTITION BY RANGE (timestamp);

-- Create monthly partitions (automated partitioning would be handled by a cron job)
CREATE TABLE audit_log_2024_12 PARTITION OF audit_log
    FOR VALUES FROM ('2024-12-01') TO ('2025-01-01');

CREATE TABLE audit_log_2025_01 PARTITION OF audit_log
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE INDEX idx_audit_log_actor ON audit_log (actor_id, timestamp DESC);
CREATE INDEX idx_audit_log_resource ON audit_log (resource_type, resource_id, timestamp DESC);

-- ============================================================================
-- MATERIALIZED VIEWS FOR REPORTING
-- ============================================================================

-- Catalog availability view (refreshed periodically)
CREATE MATERIALIZED VIEW catalog_availability AS
SELECT
    i.id,
    i.isbn,
    i.title,
    i.author,
    i.total_copies,
    i.available,
    COALESCE(co.checked_out, 0) AS checked_out,
    COALESCE(r.reserved, 0) AS reserved,
    i.status
FROM items i
LEFT JOIN (
    SELECT item_id, COUNT(*) as checked_out
    FROM checkouts
    WHERE return_date IS NULL AND status = 'active'
    GROUP BY item_id
) co ON i.id = co.item_id
LEFT JOIN (
    SELECT item_id, COUNT(*) as reserved
    FROM reservations
    WHERE status = 'pending'
    GROUP BY item_id
) r ON i.id = r.item_id
WHERE i.status = 'active';

CREATE UNIQUE INDEX idx_catalog_availability_id ON catalog_availability (id);
CREATE INDEX idx_catalog_availability_search ON catalog_availability
    USING gin (to_tsvector('english', title || ' ' || author));

-- Member activity summary
CREATE MATERIALIZED VIEW member_activity AS
SELECT
    m.id,
    m.email,
    m.name,
    m.status,
    m.fine_balance,
    COALESCE(co.active_checkouts, 0) AS active_checkouts,
    COALESCE(co.total_checkouts, 0) AS lifetime_checkouts,
    COALESCE(co.overdue_items, 0) AS overdue_items
FROM members m
LEFT JOIN (
    SELECT
        member_id,
        COUNT(*) FILTER (WHERE status = 'active') AS active_checkouts,
        COUNT(*) AS total_checkouts,
        COUNT(*) FILTER (WHERE status = 'active' AND due_date < NOW()) AS overdue_items
    FROM checkouts
    GROUP BY member_id
) co ON m.id = co.member_id;

CREATE UNIQUE INDEX idx_member_activity_id ON member_activity (id);

-- ============================================================================
-- FUNCTIONS AND TRIGGERS
-- ============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply to all tables with updated_at
CREATE TRIGGER update_items_updated_at BEFORE UPDATE ON items
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_checkouts_updated_at BEFORE UPDATE ON checkouts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_members_updated_at BEFORE UPDATE ON members
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Function to prevent negative availability
CREATE OR REPLACE FUNCTION check_item_availability()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.available < 0 THEN
        RAISE EXCEPTION 'Available copies cannot be negative for item %', NEW.id;
    END IF;
    IF NEW.available > NEW.total_copies THEN
        RAISE EXCEPTION 'Available copies cannot exceed total copies for item %', NEW.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_item_availability BEFORE INSERT OR UPDATE ON items
    FOR EACH ROW EXECUTE FUNCTION check_item_availability();

-- ============================================================================
-- ROW LEVEL SECURITY (RLS)
-- ============================================================================

ALTER TABLE members ENABLE ROW LEVEL SECURITY;

-- Policy: Members can only see their own data
CREATE POLICY member_isolation_policy ON members
    FOR ALL
    USING (
        id = current_setting('app.current_member_id', TRUE)::uuid
        OR current_setting('app.current_role', TRUE) = 'librarian'
    );

-- Policy for checkouts
ALTER TABLE checkouts ENABLE ROW LEVEL SECURITY;

CREATE POLICY checkout_isolation_policy ON checkouts
    FOR ALL
    USING (
        member_id = current_setting('app.current_member_id', TRUE)::uuid
        OR current_setting('app.current_role', TRUE) = 'librarian'
    );

-- ============================================================================
-- PERFORMANCE MONITORING
-- ============================================================================

-- View for slow queries monitoring
CREATE VIEW slow_queries AS
SELECT
    query,
    calls,
    total_exec_time,
    mean_exec_time,
    max_exec_time
FROM pg_stat_statements
WHERE mean_exec_time > 100 -- queries taking > 100ms average
ORDER BY mean_exec_time DESC
LIMIT 50;

-- ============================================================================
-- SAMPLE DATA FOR TESTING
-- ============================================================================

-- Insert sample items
INSERT INTO items (isbn, title, author, publisher, published_year, total_copies, available) VALUES
('9780141439518', 'Pride and Prejudice', 'Jane Austen', 'Penguin Classics', 1813, 5, 5),
('9780743273565', 'The Great Gatsby', 'F. Scott Fitzgerald', 'Scribner', 1925, 3, 3),
('9780061120084', 'To Kill a Mockingbird', 'Harper Lee', 'Harper Perennial', 1960, 4, 4),
('9780544003415', 'The Lord of the Rings', 'J.R.R. Tolkien', 'Mariner Books', 1954, 6, 6),
('9780142437230', '1984', 'George Orwell', 'Penguin Books', 1949, 5, 5);

-- Grant permissions
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO libranexus_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO libranexus_app;