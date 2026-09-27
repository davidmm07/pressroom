-- +goose Up

-- A slice of the storefront's schema so the tools have real rows to read and
-- write. In production these live in the order service; Pressroom reads a
-- replica and writes through its APIs (see tools.Commerce).
CREATE TABLE storefront_orders (
    id              text PRIMARY KEY,
    customer_email  text NOT NULL,
    customer_name   text NOT NULL,
    product         text NOT NULL,
    quantity        integer NOT NULL,
    status          text NOT NULL,
    total_cents     integer NOT NULL,
    carrier         text,
    tracking_number text,
    proof_status    text NOT NULL DEFAULT 'NOT_SENT',
    placed_at       timestamptz NOT NULL,
    shipped_at      timestamptz
);
CREATE INDEX storefront_orders_customer_idx ON storefront_orders (customer_email, placed_at DESC);

-- Side effects requested by agents: proofs, drafts, refunds, promo codes,
-- emails. The unique idempotency key makes a replayed tool call a no-op.
CREATE TABLE storefront_actions (
    id              text PRIMARY KEY,
    kind            text NOT NULL,
    idempotency_key text NOT NULL UNIQUE,
    payload         jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE storefront_actions;
DROP TABLE storefront_orders;
