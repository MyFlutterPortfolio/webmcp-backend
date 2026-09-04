package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/domain/business"
	"webmcp-backend/internal/domain/core"
)

type BusinessRepository struct {
	Pool *pgxpool.Pool
}

func (r BusinessRepository) GetSnapshot(ctx context.Context, organizationID, businessID core.ID) (business.Snapshot, error) {
	return WithTenantTxResult(ctx, r.Pool, organizationID, func(ctx context.Context, tx pgx.Tx) (business.Snapshot, error) {
		var snapshot business.Snapshot
		snapshot.BusinessID = businessID
		if err := tx.QueryRow(ctx, `SELECT state_version FROM businesses WHERE id = $1`, businessID).Scan(&snapshot.Version); err != nil {
			if err == pgx.ErrNoRows {
				return business.Snapshot{}, ErrBusinessNotFound
			}
			return business.Snapshot{}, fmt.Errorf("read business: %w", err)
		}

		products, err := tx.Query(ctx, `
			SELECT id, sku, name, price_cents, cost_cents, inventory_units, active, version
			FROM products WHERE business_id = $1 ORDER BY id`, businessID)
		if err != nil {
			return business.Snapshot{}, fmt.Errorf("query products: %w", err)
		}
		for products.Next() {
			var item business.Product
			if err := products.Scan(&item.ID, &item.SKU, &item.Name, &item.PriceCents, &item.CostCents, &item.InventoryUnits, &item.Active, &item.Version); err != nil {
				products.Close()
				return business.Snapshot{}, fmt.Errorf("scan product: %w", err)
			}
			snapshot.Products = append(snapshot.Products, item)
		}
		if err := products.Err(); err != nil {
			products.Close()
			return business.Snapshot{}, fmt.Errorf("iterate products: %w", err)
		}
		products.Close()

		customers, err := tx.Query(ctx, `
			SELECT id, name, segment, active, version
			FROM customers WHERE business_id = $1 ORDER BY id`, businessID)
		if err != nil {
			return business.Snapshot{}, fmt.Errorf("query customers: %w", err)
		}
		for customers.Next() {
			var item business.Customer
			if err := customers.Scan(&item.ID, &item.Name, &item.Segment, &item.Active, &item.Version); err != nil {
				customers.Close()
				return business.Snapshot{}, fmt.Errorf("scan customer: %w", err)
			}
			snapshot.Customers = append(snapshot.Customers, item)
		}
		if err := customers.Err(); err != nil {
			customers.Close()
			return business.Snapshot{}, fmt.Errorf("iterate customers: %w", err)
		}
		customers.Close()

		orders, err := tx.Query(ctx, `
			SELECT id, customer_id, product_id, quantity, revenue_cents, cost_cents,
			       EXTRACT(EPOCH FROM occurred_at)::bigint, version
			FROM orders WHERE business_id = $1 ORDER BY occurred_at DESC, id`, businessID)
		if err != nil {
			return business.Snapshot{}, fmt.Errorf("query orders: %w", err)
		}
		for orders.Next() {
			var item business.Order
			if err := orders.Scan(&item.ID, &item.CustomerID, &item.ProductID, &item.Quantity, &item.RevenueCents, &item.CostCents, &item.OccurredAtUnix, &item.Version); err != nil {
				orders.Close()
				return business.Snapshot{}, fmt.Errorf("scan order: %w", err)
			}
			snapshot.Orders = append(snapshot.Orders, item)
		}
		if err := orders.Err(); err != nil {
			orders.Close()
			return business.Snapshot{}, fmt.Errorf("iterate orders: %w", err)
		}
		orders.Close()

		metrics, err := tx.Query(ctx, `
			SELECT metric_key, value, unit, period, source, version
			FROM metrics WHERE business_id = $1 ORDER BY metric_key, period`, businessID)
		if err != nil {
			return business.Snapshot{}, fmt.Errorf("query metrics: %w", err)
		}
		for metrics.Next() {
			var item business.Metric
			if err := metrics.Scan(&item.Key, &item.Value, &item.Unit, &item.Period, &item.Source, &item.Version); err != nil {
				metrics.Close()
				return business.Snapshot{}, fmt.Errorf("scan metric: %w", err)
			}
			snapshot.Metrics = append(snapshot.Metrics, item)
		}
		if err := metrics.Err(); err != nil {
			metrics.Close()
			return business.Snapshot{}, fmt.Errorf("iterate metrics: %w", err)
		}
		metrics.Close()
		return snapshot, nil
	})
}
