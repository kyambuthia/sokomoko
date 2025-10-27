CREATE TABLE Users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    phonenumber TEXT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user'
);

CREATE TABLE Products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    partner_id INTEGER,
    name TEXT NOT NULL,
    description TEXT,
    price REAL NOT NULL,
    category TEXT,
    stock_quantity INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (partner_id) REFERENCES Users(id)
);

CREATE TABLE Orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    order_date DATETIME DEFAULT CURRENT_TIMESTAMP,
    total_amount REAL NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    FOREIGN KEY (user_id) REFERENCES Users(id)
);

CREATE TABLE Order_Items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    quantity INTEGER NOT NULL,
    price_at_order REAL NOT NULL,
    FOREIGN KEY (order_id) REFERENCES Orders(id),
    FOREIGN KEY (product_id) REFERENCES Products(id)
);

CREATE TABLE Delivery (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id INTEGER NOT NULL UNIQUE,
    delivery_address TEXT NOT NULL,
    delivery_status TEXT NOT NULL DEFAULT 'preparing',
    tracking_number TEXT UNIQUE,
    estimated_delivery_date DATETIME,
    actual_delivery_date DATETIME,
    FOREIGN KEY (order_id) REFERENCES Orders(id)
);

CREATE TABLE Blog_Posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    author TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    content TEXT
);