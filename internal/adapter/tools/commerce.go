package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Order is the storefront's view of an order.
type Order struct {
	ID             string     `json:"orderId"`
	CustomerEmail  string     `json:"customerEmail"`
	CustomerName   string     `json:"customerName"`
	Product        string     `json:"product"`
	Quantity       int        `json:"quantity"`
	Status         string     `json:"status"`
	TotalCents     int        `json:"totalCents"`
	Carrier        string     `json:"carrier,omitempty"`
	TrackingNumber string     `json:"trackingNumber,omitempty"`
	ProofStatus    string     `json:"proofStatus"`
	PlacedAt       time.Time  `json:"placedAt"`
	ShippedAt      *time.Time `json:"shippedAt,omitempty"`
}

// Customer summarises a customer's order history.
type Customer struct {
	Email              string    `json:"customerEmail"`
	Name               string    `json:"customerName"`
	LifetimeOrders     int       `json:"lifetimeOrders"`
	LifetimeSpendCents int       `json:"lifetimeSpendCents"`
	LastProduct        string    `json:"product"`
	LastOrderAt        time.Time `json:"lastOrderAt"`
	DaysSinceLastOrder int       `json:"daysSinceLastOrder"`
}

// ErrNotFound is returned for unknown orders and customers.
var ErrNotFound = errors.New("not found")

// Commerce is the storefront the tools read and write: in production the
// order service's read replica and write APIs. Writes take an idempotency
// key (see port.ToolCallInfo) and return the ID of the record they created,
// or of the one a previous attempt with the same key already created.
type Commerce interface {
	Order(ctx context.Context, id string) (Order, error)
	Customer(ctx context.Context, email string) (Customer, error)
	Record(ctx context.Context, kind, key string, payload any) (id string, err error)
}

// PGCommerce keeps the storefront demo tables in Pressroom's own database so
// the project runs without the real order service.
type PGCommerce struct {
	pool *pgxpool.Pool
}

func NewPGCommerce(pool *pgxpool.Pool) *PGCommerce { return &PGCommerce{pool: pool} }

func (c *PGCommerce) Order(ctx context.Context, id string) (Order, error) {
	var o Order
	err := c.pool.QueryRow(ctx, `
		SELECT id, customer_email, customer_name, product, quantity, status, total_cents,
		       coalesce(carrier, ''), coalesce(tracking_number, ''), proof_status, placed_at, shipped_at
		FROM storefront_orders WHERE id = $1`, id).
		Scan(&o.ID, &o.CustomerEmail, &o.CustomerName, &o.Product, &o.Quantity, &o.Status, &o.TotalCents,
			&o.Carrier, &o.TrackingNumber, &o.ProofStatus, &o.PlacedAt, &o.ShippedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, fmt.Errorf("order %s: %w", id, ErrNotFound)
	}
	return o, err
}

func (c *PGCommerce) Customer(ctx context.Context, email string) (Customer, error) {
	cu := Customer{Email: email}
	err := c.pool.QueryRow(ctx, `
		SELECT customer_name, count(*) OVER (), sum(total_cents) OVER (), product, placed_at
		FROM storefront_orders WHERE customer_email = $1
		ORDER BY placed_at DESC LIMIT 1`, email).
		Scan(&cu.Name, &cu.LifetimeOrders, &cu.LifetimeSpendCents, &cu.LastProduct, &cu.LastOrderAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, fmt.Errorf("customer %s: %w", email, ErrNotFound)
	}
	cu.DaysSinceLastOrder = int(time.Since(cu.LastOrderAt).Hours() / 24)
	return cu, err
}

func (c *PGCommerce) Record(ctx context.Context, kind, key string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var id string
	// The no-op update makes RETURNING yield the existing row on a retry.
	err = c.pool.QueryRow(ctx, `
		INSERT INTO storefront_actions (id, kind, idempotency_key, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (idempotency_key) DO UPDATE SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id`, newRef(kind), kind, key, body).Scan(&id)
	return id, err
}

func newRef(kind string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	prefix := map[string]string{"PROOF": "pf", "DRAFT_REPLY": "dr", "REFUND": "re", "PROMO": "pr", "EMAIL": "em"}[kind]
	if prefix == "" {
		prefix = "ac"
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
