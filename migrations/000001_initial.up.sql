-- WebMCP canonical schema. Every tenant-owned table carries organization_id
-- so ownership can be enforced both in application code and PostgreSQL RLS.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE FUNCTION set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE TABLE organizations (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    role text NOT NULL CHECK (role IN ('owner', 'operator', 'viewer')),
    display_name text NOT NULL CHECK (length(trim(display_name)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE TABLE businesses (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    name text NOT NULL CHECK (length(trim(name)) > 0),
    state_version bigint NOT NULL DEFAULT 1 CHECK (state_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE TABLE products (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    sku text NOT NULL,
    name text NOT NULL,
    price_cents bigint NOT NULL CHECK (price_cents >= 0),
    cost_cents bigint NOT NULL CHECK (cost_cents >= 0 AND cost_cents <= price_cents),
    inventory_units bigint NOT NULL CHECK (inventory_units >= 0),
    active boolean NOT NULL DEFAULT true,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    UNIQUE (business_id, sku),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id)
);

CREATE TABLE customers (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    name text NOT NULL,
    segment text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id)
);

CREATE TABLE orders (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    customer_id text NOT NULL,
    product_id text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    revenue_cents bigint NOT NULL CHECK (revenue_cents >= 0),
    cost_cents bigint NOT NULL CHECK (cost_cents >= 0 AND cost_cents <= revenue_cents),
    occurred_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
    FOREIGN KEY (organization_id, customer_id) REFERENCES customers(organization_id, id),
    FOREIGN KEY (organization_id, product_id) REFERENCES products(organization_id, id)
);

CREATE TABLE metrics (
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    metric_key text NOT NULL,
    period text NOT NULL,
    value double precision NOT NULL,
    unit text NOT NULL,
    source text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, business_id, metric_key, period),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id)
);

CREATE TABLE goals (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    objective text NOT NULL CHECK (length(trim(objective)) > 0),
    time_horizon_days integer NOT NULL CHECK (time_horizon_days > 0),
    preference text NOT NULL CHECK (preference IN ('conservative', 'balanced', 'aggressive')),
    status text NOT NULL CHECK (status IN ('draft', 'active', 'completed', 'cancelled')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
    FOREIGN KEY (organization_id, created_by) REFERENCES users(organization_id, id)
);

CREATE TABLE goal_constraints (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    goal_id text NOT NULL,
    key text NOT NULL,
    value text NOT NULL,
    hard boolean NOT NULL DEFAULT true,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (goal_id, key),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, goal_id) REFERENCES goals(organization_id, id)
);

CREATE TABLE scenarios (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    goal_id text NOT NULL,
    base_business_version bigint NOT NULL CHECK (base_business_version > 0),
    name text NOT NULL,
    objective text NOT NULL,
    proposed_actions jsonb NOT NULL DEFAULT '[]'::jsonb,
    result jsonb NOT NULL DEFAULT '{}'::jsonb,
    risks jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL CHECK (status IN ('draft', 'simulated', 'compared', 'archived')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
      UNIQUE (organization_id, id),
      FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
      FOREIGN KEY (organization_id, goal_id) REFERENCES goals(organization_id, id),
      FOREIGN KEY (organization_id, created_by) REFERENCES users(organization_id, id)
  );

CREATE TABLE proposals (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    goal_id text NOT NULL,
    current_version_number integer NOT NULL DEFAULT 1 CHECK (current_version_number > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
    FOREIGN KEY (organization_id, goal_id) REFERENCES goals(organization_id, id)
);

CREATE TABLE proposal_versions (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    proposal_id text NOT NULL,
    number integer NOT NULL CHECK (number > 0),
    scenario_id text NOT NULL,
    base_business_version bigint NOT NULL CHECK (base_business_version > 0),
    summary text NOT NULL,
    changes jsonb NOT NULL DEFAULT '[]'::jsonb,
    status text NOT NULL CHECK (status IN ('draft', 'in_review', 'approved', 'rejected', 'committed', 'superseded')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (proposal_id, number),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, proposal_id) REFERENCES proposals(organization_id, id),
    FOREIGN KEY (organization_id, scenario_id) REFERENCES scenarios(organization_id, id)
);

CREATE TABLE approvals (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    proposal_id text NOT NULL,
    proposal_version_id text NOT NULL,
    decided_by text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'revoked')),
    reason text NOT NULL DEFAULT '',
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
    FOREIGN KEY (organization_id, proposal_id) REFERENCES proposals(organization_id, id),
    FOREIGN KEY (organization_id, proposal_version_id) REFERENCES proposal_versions(organization_id, id),
    FOREIGN KEY (organization_id, decided_by) REFERENCES users(organization_id, id)
);

CREATE TABLE commit_operations (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    proposal_id text NOT NULL,
    proposal_version_id text NOT NULL,
    idempotency_key text NOT NULL,
    expected_business_version bigint NOT NULL CHECK (expected_business_version > 0),
    status text NOT NULL CHECK (status IN ('requested', 'succeeded', 'failed')),
    committed_business_version bigint CHECK (committed_business_version IS NULL OR committed_business_version > 0),
    error_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (organization_id, idempotency_key),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
    FOREIGN KEY (organization_id, proposal_id) REFERENCES proposals(organization_id, id),
    FOREIGN KEY (organization_id, proposal_version_id) REFERENCES proposal_versions(organization_id, id)
);

CREATE TABLE committed_decisions (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    business_id text NOT NULL,
    proposal_id text NOT NULL,
    proposal_version_id text NOT NULL,
    committed_business_version bigint NOT NULL CHECK (committed_business_version > 0),
    summary text NOT NULL,
    committed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, proposal_version_id),
    FOREIGN KEY (organization_id, business_id) REFERENCES businesses(organization_id, id),
    FOREIGN KEY (organization_id, proposal_id) REFERENCES proposals(organization_id, id),
    FOREIGN KEY (organization_id, proposal_version_id) REFERENCES proposal_versions(organization_id, id)
);

CREATE TABLE audit_events (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    actor_type text NOT NULL CHECK (actor_type IN ('human', 'agent', 'system')),
    actor_id text NOT NULL,
    action text NOT NULL,
    aggregate text NOT NULL,
    aggregate_id text NOT NULL,
    request_id text NOT NULL,
    operation_id text,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (organization_id, id)
);

CREATE INDEX idx_businesses_organization ON businesses(organization_id);
CREATE INDEX idx_products_business ON products(business_id);
CREATE INDEX idx_customers_business ON customers(business_id);
CREATE INDEX idx_orders_business_occurred ON orders(business_id, occurred_at DESC);
CREATE INDEX idx_goals_business_status ON goals(business_id, status);
CREATE INDEX idx_scenarios_goal_status ON scenarios(goal_id, status);
CREATE INDEX idx_proposals_business ON proposals(business_id, updated_at DESC);
CREATE INDEX idx_approvals_version_status ON approvals(proposal_version_id, status);
CREATE INDEX idx_commit_operations_business ON commit_operations(business_id, created_at DESC);
CREATE INDEX idx_audit_aggregate ON audit_events(organization_id, aggregate, aggregate_id, occurred_at DESC);

CREATE TRIGGER organizations_updated_at BEFORE UPDATE ON organizations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER businesses_updated_at BEFORE UPDATE ON businesses FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER products_updated_at BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER customers_updated_at BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER goals_updated_at BEFORE UPDATE ON goals FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER scenarios_updated_at BEFORE UPDATE ON scenarios FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER proposals_updated_at BEFORE UPDATE ON proposals FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER approvals_updated_at BEFORE UPDATE ON approvals FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The application transaction sets this local setting before any tenant query.
-- Empty/missing context denies access rather than widening it.
DO $$
DECLARE table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['users','businesses','products','customers','orders','metrics','goals','goal_constraints','scenarios','proposals','proposal_versions','approvals','commit_operations','committed_decisions','audit_events'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
        EXECUTE format('CREATE POLICY %I_tenant_isolation ON %I USING (organization_id = current_setting(''app.organization_id'', true)) WITH CHECK (organization_id = current_setting(''app.organization_id'', true))', table_name, table_name);
    END LOOP;
END $$;
