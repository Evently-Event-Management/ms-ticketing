-- Sample data for testing purposes
INSERT INTO orders (
    user_id,
    event_id,
    session_id,
    organization_id,
    status,
    subtotal,
    discount_id,
    discount_code,
    discount_amount,
    price,
    created_at
) VALUES (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',  -- user_id
    'cccccccc-cccc-cccc-cccc-cccccccccccc',  -- event_id
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',  -- session_id
    'dddddddd-dddd-dddd-dddd-dddddddddddd',  -- organization_id
    'completed',                             -- status
    300.00,                                  -- subtotal
    '55555555-5555-5555-5555-555555555555',  -- discount_id
    'WELCOME20',                             -- discount_code
    50.00,                                   -- discount_amount
    250.00,                                  -- price
    NOW()                                    -- created_at
);

-- Get the last inserted order_id to use for tickets
DO $$
DECLARE
    last_order_id UUID;
BEGIN
    SELECT order_id INTO last_order_id FROM orders ORDER BY created_at DESC LIMIT 1;
    
    -- Insert sample tickets associated with the last order
    INSERT INTO tickets (
        order_id,
        seat_id,
        seat_label,
        colour,
        tier_id,
        tier_name,
        qr_code,
        price_at_purchase,
        issued_at
    ) VALUES 
    (
        last_order_id,                           -- order_id
        '11111111-1111-1111-1111-111111111111',  -- seat_id
        'A1',                                    -- seat_label
        'Red',                                   -- colour
        '33333333-3333-3333-3333-333333333333',  -- tier_id
        'VIP',                                   -- tier_name
        decode('48656c6c6f20515221', 'hex'),     -- qr_code (Hello QR!)
        150.00,                                  -- price_at_purchase
        NOW()                                    -- issued_at
    ),
    (
        last_order_id,                           -- order_id
        '22222222-2222-2222-2222-222222222222',  -- seat_id
        'A2',                                    -- seat_label
        'Blue',                                  -- colour
        '44444444-4444-4444-4444-444444444444',  -- tier_id
        'Standard',                              -- tier_name
        decode('48656c6c6f20515221', 'hex'),     -- qr_code (Hello QR!)
        100.00,                                  -- price_at_purchase
        NOW()                                    -- issued_at
    );
END $$;