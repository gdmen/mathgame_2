CREATE TABLE IF NOT EXISTS user_playlist (
    user_id BIGINT UNSIGNED NOT NULL,
    playlist_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (user_id, playlist_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (playlist_id) REFERENCES playlists(id)
) DEFAULT CHARSET=utf8mb4;
