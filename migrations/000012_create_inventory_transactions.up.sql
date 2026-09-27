CREATE TABLE inventory_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inventory_id UUID NOT NULL REFERENCES inventories(id),
    type VARCHAR(30) NOT NULL CHECK (type IN ('STOCK_IN', 'STOCK_OUT', 'RESERVE', 'RELEASE', 'ADJUSTMENT')),
    quantity INTEGER NOT NULL,
    reference_type VARCHAR(50),
    reference_id UUID,
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX idx_inventory_transactions_inventory_id ON inventory_transactions (inventory_id);
CREATE INDEX idx_inventory_transactions_reference ON inventory_transactions (reference_type, reference_id);
