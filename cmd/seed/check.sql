
SELECT
    screen_id,
    starts_at AT TIME ZONE 'Asia/Kolkata' AS local_start,
    ends_at AT TIME ZONE 'Asia/Kolkata' AS local_end
FROM showtimes
ORDER BY starts_at, screen_id
LIMIT 15;