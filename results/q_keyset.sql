SELECT id, title, content, created_at, updated_at FROM notes WHERE (created_at, id) < ('2025-11-23 17:46:43.602009+03', 500001) ORDER BY created_at DESC, id DESC LIMIT 20
