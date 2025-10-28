## SOKOMOKO 
===

sokomoko is a small E-Commerce website written in go, HTML, CSS, JS and sqlite3 for the DB - no frameworks used.
this project is a learning adventure through the tools, languages, techniques used when building modern web applications the main constraint here being the use of bare languages.

## REQUIREMENTS 
===
the requirements for this project are as listed below.
- Go
- Sqlite3


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