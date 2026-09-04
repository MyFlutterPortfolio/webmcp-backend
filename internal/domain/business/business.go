package business

import (
	"fmt"
	"strings"

	"webmcp-backend/internal/domain/core"
)

type Product struct {
	ID             core.ID
	SKU            string
	Name           string
	PriceCents     int64
	CostCents      int64
	InventoryUnits int64
	Active         bool
	Version        uint64
}

func (p Product) Validate() error {
	if err := p.ID.Validate("product_id"); err != nil {
		return err
	}
	if strings.TrimSpace(p.SKU) == "" || strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("product sku and name are required")
	}
	if p.PriceCents < 0 || p.CostCents < 0 || p.InventoryUnits < 0 {
		return fmt.Errorf("product monetary values and inventory cannot be negative")
	}
	if p.CostCents > p.PriceCents {
		return fmt.Errorf("product cost cannot exceed price")
	}
	if p.Version == 0 {
		return fmt.Errorf("product version must be positive")
	}
	return nil
}

type Customer struct {
	ID      core.ID
	Name    string
	Segment string
	Active  bool
	Version uint64
}

func (c Customer) Validate() error {
	if err := c.ID.Validate("customer_id"); err != nil {
		return err
	}
	if strings.TrimSpace(c.Name) == "" || c.Version == 0 {
		return fmt.Errorf("customer name and positive version are required")
	}
	return nil
}

type Order struct {
	ID             core.ID
	CustomerID     core.ID
	ProductID      core.ID
	Quantity       int64
	RevenueCents   int64
	CostCents      int64
	OccurredAtUnix int64
	Version        uint64
}

func (o Order) Validate() error {
	if err := o.ID.Validate("order_id"); err != nil {
		return err
	}
	if err := o.CustomerID.Validate("customer_id"); err != nil {
		return err
	}
	if err := o.ProductID.Validate("product_id"); err != nil {
		return err
	}
	if o.Quantity <= 0 || o.RevenueCents < 0 || o.CostCents < 0 || o.Version == 0 {
		return fmt.Errorf("order quantity, money and version are invalid")
	}
	if o.CostCents > o.RevenueCents {
		return fmt.Errorf("order cost cannot exceed revenue")
	}
	return nil
}

type Metric struct {
	Key     string
	Value   float64
	Unit    string
	Period  string
	Source  string
	Version uint64
}

func (m Metric) Validate() error {
	if strings.TrimSpace(m.Key) == "" || strings.TrimSpace(m.Period) == "" || strings.TrimSpace(m.Source) == "" || m.Version == 0 {
		return fmt.Errorf("metric key, period, source and positive version are required")
	}
	return nil
}

type Snapshot struct {
	BusinessID core.ID
	Version    uint64
	Products   []Product
	Customers  []Customer
	Orders     []Order
	Metrics    []Metric
}

func (s Snapshot) Validate() error {
	if err := s.BusinessID.Validate("business_id"); err != nil {
		return err
	}
	if s.Version == 0 {
		return fmt.Errorf("business version must be positive")
	}
	for _, item := range s.Products {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range s.Customers {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range s.Orders {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range s.Metrics {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}
