CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    salt TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user', -- 'user', 'admin'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    parent_id INTEGER, -- For hierarchical categories
    FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    price REAL NOT NULL,
    stock_quantity INTEGER NOT NULL DEFAULT 0,
    category_id INTEGER,
    image_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL
);

-- Optional: For future order management
-- CREATE TABLE IF NOT EXISTS orders (
--     id INTEGER PRIMARY KEY AUTOINCREMENT,
--     user_id INTEGER NOT NULL,
--     order_date DATETIME DEFAULT CURRENT_TIMESTAMP,
--     total_amount REAL NOT NULL,
--     status TEXT NOT NULL DEFAULT 'pending', -- e.g., 'pending', 'completed', 'cancelled'
--     FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
-- );

-- CREATE TABLE IF NOT EXISTS order_items (
--     id INTEGER PRIMARY KEY AUTOINCREMENT,
--     order_id INTEGER NOT NULL,
--     product_id INTEGER NOT NULL,
--     quantity INTEGER NOT NULL,
--     price_at_purchase REAL NOT NULL,
--     FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
--     FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
-- );
