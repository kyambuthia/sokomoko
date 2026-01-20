## SOKOMOKO 
===

sokomoko is a small E-Commerce website written in Go, HTML, CSS, JS and sqlite3 for the DB - no frameworks used.
this project is a learning adventure through the tools, languages, techniques used when building modern web applications the main constraint here being the use of bare languages.

## Project Structure
===
Following Go best practices, the project is organized as follows:
- `cmd/`: Main entry points for the application.
    - `sokomoko/`: The main web server.
- `internal/`: Private library code.
    - `db/`: Database models and operations.
    - `routes/`: HTTP handlers.
    - `auth/`: Authentication logic.
    - `ui/`: Embedded templates and static assets.
- `bin/`: Compiled binaries.
- `db/`: Database files.

## REQUIREMENTS 
===
the requirements for this project are as listed below.
- Go (version 1.24 or higher)
- Sqlite3


## Setup and Running
1.  **Install Go:** Ensure you have Go installed.
2.  **Clone the repository:**
    ```bash
    git clone github.com/kyambuthia/sokomoko.git
    cd sokomoko
    ```
3.  **Manage Dependencies:**
    > [!IMPORTANT]
    > Always use `go` commands to manage dependencies.
    ```bash
    go mod tidy
    ```
4.  **Run the application:**
    ```bash
    go run ./cmd/sokomoko
    ```
5.  **Build the application:**
    ```bash
    go build -o bin/sokomoko ./cmd/sokomoko
    ```

The application will automatically create and migrate the database (`db/t.db`) in the root directory on first run.


## CONTRIBUTING
===============
if you spot a bug, Warn a brother - Open an issue to contribute to the project.