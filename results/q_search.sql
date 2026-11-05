SELECT id, title FROM notes WHERE to_tsvector('simple', title) @@ plainto_tsquery('simple', '424242') ORDER BY id DESC LIMIT 20
