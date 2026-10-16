
SELECT
    st.id AS showtime_id,
    sc.name AS screen,
    COUNT(ss.seat_id) AS seat_count
FROM showtimes st
JOIN screens sc ON sc.id = st.screen_id
LEFT JOIN showtime_seats ss ON ss.showtime_id = st.id
GROUP BY st.id, sc.name
HAVING COUNT(ss.seat_id) <> 50;