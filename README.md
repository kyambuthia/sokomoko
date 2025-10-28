## SOKOMOKO 
=========================
sokomoko is a small E-Commerce website written in go, HTML, CSS, JS and sqlite3 for the DB - no frameworks used.


## REQUIREMENTS 
========
Requires GO and a SQLITE3

clone the repository

`git clone github.com\kyambuthia\sokomoko.git`


## Setup and Database Initialization
1.  **Install Go:** Ensure you have Go installed (version 1.24 or higher recommended).
2.  **Clone the repository:**
    ```bash
    git clone github.com/kyambuthia/sokomoko.git
    cd sokomoko
    ```
3.  **Install dependencies:**
    ```bash
    go mod tidy
    ```
4.  **Run the application:**
    ```bash
    go run src/srvr.go
    ```
    The application will automatically create and migrate the database (`db/t.db`) on first run.


## CONTRIBUTING
===============
if you spot a bug, Warn a brother - Open an issue to contribute to the project.
