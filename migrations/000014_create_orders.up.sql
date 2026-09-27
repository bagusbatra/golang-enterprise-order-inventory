CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number VARCHAR(50) UNIQUE NOT NULL,
    customer_id UUID NOT NULL REFERENCES users(id),
    warehouse_id UUID NOT NULL REFERENCES warehouses(id),
    status VARCHAR(30) NOT NULL CHECK (status IN ('PENDING', 'WAITING_PAYMENT', 'PAID', 'PROCESSING', 'PACKED', 'SHIPPED', 'COMPLETED', 'CANCELLED', 'EXPIRED')),
    subtotal NUMERIC(15,2) NOT NULL,
    discount NUMERIC(15,2) NOT NULL DEFAULT 0,
    tax NUMERIC(15,2) NOT NULL,
    shipping_cost NUMERIC(15,2) NOT NULL,
    grand_total NUMERIC(15,2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_customer_id ON orders (customer_id);
CREATE INDEX idx_orders_status ON orders (status);
CREATE INDEX idx_orders_created_at ON orders (created_at);
CREATE INDEX idx_orders_customer_created ON orders (customer_id, created_at);
CREATE INDEX idx_orders_status_created ON orders (status, created_at);
