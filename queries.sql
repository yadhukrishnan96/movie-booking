
SELECT
    s.id AS seat_id,
    s.seat_number,
    CASE
        WHEN ss.status = 'held'
             AND ss.hold_expires_at <= NOW()
        THEN 'available'
        ELSE ss.status
    END AS status
FROM showtimes st
JOIN seats s
    ON s.screen_id = st.screen_id
JOIN showtime_seats ss
    ON ss.showtime_id = st.id
   AND ss.seat_id = s.id
WHERE st.id = $1
ORDER BY s.seat_number;