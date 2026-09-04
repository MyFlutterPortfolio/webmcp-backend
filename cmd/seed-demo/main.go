package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"webmcp-backend/internal/domain/core"
)

// seedSpec is deliberately explicit. The command is intended for a dedicated
// judge database, not for silently adding demo data to an arbitrary database.
type seedSpec struct {
	databaseURL    string
	userID         core.ID
	organizationID core.ID
	businessID     core.ID
	goalID         core.ID
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	spec, err := loadSpec()
	if err != nil {
		logger.Error("demo seed failed", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, spec.databaseURL)
	if err != nil {
		logger.Error("demo database connection failed", "error", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())
	if err := seed(ctx, conn, spec); err != nil {
		logger.Error("demo seed failed", "error", err)
		os.Exit(1)
	}
	logger.Info("demo seed ready", "business_id", spec.businessID, "goal_id", spec.goalID)
}

func loadSpec() (seedSpec, error) {
	spec := seedSpec{databaseURL: strings.TrimSpace(os.Getenv("WEBMCP_DATABASE_URL"))}
	if spec.databaseURL == "" {
		return seedSpec{}, fmt.Errorf("WEBMCP_DATABASE_URL is required")
	}
	var err error
	if spec.userID, err = requiredID("WEBMCP_GUEST_DEMO_USER_ID"); err != nil {
		return seedSpec{}, err
	}
	if spec.organizationID, err = requiredID("WEBMCP_GUEST_DEMO_ORGANIZATION_ID"); err != nil {
		return seedSpec{}, err
	}
	if spec.businessID, err = requiredID("WEBMCP_GUEST_DEMO_BUSINESS_ID"); err != nil {
		return seedSpec{}, err
	}
	if spec.goalID, err = requiredID("WEBMCP_GUEST_DEMO_GOAL_ID"); err != nil {
		return seedSpec{}, err
	}
	return spec, nil
}

func requiredID(key string) (core.ID, error) {
	value := core.ID(strings.TrimSpace(os.Getenv(key)))
	if err := value.Validate(key); err != nil {
		return "", err
	}
	return value, nil
}

func seed(ctx context.Context, conn *pgx.Conn, spec seedSpec) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO organizations (id, name) VALUES ($1, 'Northstar Demo Organization') ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID}},
	}
	for index, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("seed statement %d: %w", index+1, err)
		}
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, spec.organizationID); err != nil {
		return fmt.Errorf("set seed tenant scope: %w", err)
	}

	statements = []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, organization_id, role, display_name) VALUES ($1, $2, 'viewer', 'Northstar Demo Viewer') ON CONFLICT (id) DO NOTHING`, []any{spec.userID, spec.organizationID}},
		{`INSERT INTO businesses (id, organization_id, name) VALUES ($1, $2, 'Northstar Goods') ON CONFLICT (id) DO NOTHING`, []any{spec.businessID, spec.organizationID}},
		{`INSERT INTO products (id, organization_id, business_id, sku, name, price_cents, cost_cents, inventory_units, active) VALUES ('demo-product-001', $1, $2, 'NS-RET-001', 'Repeat customer bundle', 8000, 4200, 340, true) ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO products (id, organization_id, business_id, sku, name, price_cents, cost_cents, inventory_units, active) VALUES ('demo-product-002', $1, $2, 'NS-RET-002', 'Seasonal essentials', 5600, 3100, 180, true) ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO customers (id, organization_id, business_id, name, segment, active) VALUES ('demo-customer-001', $1, $2, 'Aster & Co', 'high-value repeat', true) ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO customers (id, organization_id, business_id, name, segment, active) VALUES ('demo-customer-002', $1, $2, 'Mira Studio', 'returning', true) ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO orders (id, organization_id, business_id, customer_id, product_id, quantity, revenue_cents, cost_cents, occurred_at) VALUES ('demo-order-001', $1, $2, 'demo-customer-001', 'demo-product-001', 3, 24000, 12600, now() - interval '12 days') ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO orders (id, organization_id, business_id, customer_id, product_id, quantity, revenue_cents, cost_cents, occurred_at) VALUES ('demo-order-002', $1, $2, 'demo-customer-002', 'demo-product-002', 2, 11200, 6200, now() - interval '7 days') ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO metrics (organization_id, business_id, metric_key, period, value, unit, source) VALUES ($1, $2, 'revenue_90d', '90d', 128400, 'USD', 'seeded demo snapshot') ON CONFLICT (organization_id, business_id, metric_key, period) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO metrics (organization_id, business_id, metric_key, period, value, unit, source) VALUES ($1, $2, 'gross_margin_percent', '90d', 34.8, 'percent', 'seeded demo snapshot') ON CONFLICT (organization_id, business_id, metric_key, period) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO metrics (organization_id, business_id, metric_key, period, value, unit, source) VALUES ($1, $2, 'inventory_cover_days', 'current', 21, 'days', 'seeded demo snapshot') ON CONFLICT (organization_id, business_id, metric_key, period) DO NOTHING`, []any{spec.organizationID, spec.businessID}},
		{`INSERT INTO goals (id, organization_id, business_id, objective, time_horizon_days, preference, status, created_by) VALUES ($1, $2, $3, 'Grow repeat revenue without increasing stock pressure.', 90, 'balanced', 'active', $4) ON CONFLICT (id) DO NOTHING`, []any{spec.goalID, spec.organizationID, spec.businessID, spec.userID}},
		{`INSERT INTO goal_constraints (id, organization_id, goal_id, key, value, hard, created_by) VALUES ('demo-constraint-budget', $1, $2, 'budget_cents', '8000', true, $3) ON CONFLICT (id) DO NOTHING`, []any{spec.organizationID, spec.goalID, spec.userID}},
	}
	for index, statement := range statements {
		if _, err := tx.Exec(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("seed statement %d: %w", index+1, err)
		}
	}

	var ready bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users u
			JOIN businesses b ON b.organization_id = u.organization_id
			JOIN goals g ON g.organization_id = b.organization_id AND g.business_id = b.id
			WHERE u.id = $1 AND u.organization_id = $2 AND u.role = 'viewer'
			  AND b.id = $3 AND g.id = $4 AND g.status = 'active'
		)
		AND EXISTS (SELECT 1 FROM products WHERE organization_id = $2 AND business_id = $3 AND active = true)`, spec.userID, spec.organizationID, spec.businessID, spec.goalID).Scan(&ready); err != nil {
		return fmt.Errorf("verify demo anchors: %w", err)
	}
	if !ready {
		return fmt.Errorf("demo seed verification failed; existing rows may use incompatible IDs or tenant scope")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seed transaction: %w", err)
	}
	return nil
}
