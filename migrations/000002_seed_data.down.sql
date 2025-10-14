-- Remove seed data
DELETE FROM tickets WHERE seat_id IN (
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222222'
);

DELETE FROM orders WHERE user_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa' AND event_id = 'cccccccc-cccc-cccc-cccc-cccccccccccc';