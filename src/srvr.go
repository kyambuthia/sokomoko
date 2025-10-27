package main

import (
        "fmt"
        "log"
        "html/template"
        "net/http"
        "time"
        "path/filepath"
        "os"
        "github.com/google/uuid"

        "database/sql"
        "golang.org/x/crypto/bcrypt"
        _ "github.com/ncruces/go-sqlite3/driver"
        _ "github.com/ncruces/go-sqlite3/embed"
)

type Product struct {
        Name string     `json:"name"`
        Price float64   `json:"price"`
        Category string `json:"category"`
}

type Persona struct {
	Name string		`json: "name"`
	Phonenumber string	`json: "phonenumber"`
}

type Item struct {
        Title string    `json: "title"`
        Type string     `json: "type"`
        Category string `json: "category"`
        Price float64   `json: "Price"`
        Quantity int    `json: "quantity"`
        Note string     `json: "note"`
}

type Blog struct {
        Title string `json:"title"`
        Author string `json:"author"`
        Created time.Time `json: "created"`
        content string `json: "content"`
}

// UserSession stores user data for a session
type UserSession struct {
    UserID int
    Email  string
    Role   string
}

var sessions = make(map[string]UserSession) // sessionID -> UserSession

// generateSessionID generates a unique session ID
func generateSessionID() string {
    return uuid.New().String()
}

// setSessionCookie sets a session cookie for the user
func setSessionCookie(w http.ResponseWriter, sessionID string) {
    http.SetCookie(w, &http.Cookie{
        Name:     "session_id",
        Value:    sessionID,
        Path:     "/",
        Expires:  time.Now().Add(24 * time.Hour), // Session expires in 24 hours
        HttpOnly: true, // Prevent JavaScript access to the cookie
        Secure:   false, // Set to true in production with HTTPS
        SameSite: http.SameSiteLaxMode,
    })
}

// getSessionUser retrieves the UserSession from the session cookie
func getSessionUser(req *http.Request) (UserSession, bool) {
    cookie, err := req.Cookie("session_id")
    if err != nil {
        return UserSession{}, false
    }

    sessionID := cookie.Value
    userSession, ok := sessions[sessionID]
    return userSession, ok
}

var db *sql.DB
var version string

// TemplateData holds data to be passed to HTML templates
type TemplateData struct {
    User UserSession
    IsAuthenticated bool
    Personas []Persona // Added for the index page
    // Add other common data here
}

// signupHandler handles user registration
func signupHandler(w http.ResponseWriter, req *http.Request) {
    if req.Method == "GET" {
        tmpl, err := template.ParseFiles(
            "./src/templates/layout.html",
            "./src/templates/components/nav.html",
            "./src/templates/components/footer.html",
            "./src/templates/pages/signup.html",
        )
        if err != nil {
            log.Printf("Error parsing signup templates: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        err = tmpl.ExecuteTemplate(w, "root_template", nil)
        if err != nil {
            log.Printf("Error executing signup template: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        return
    }

    if req.Method == "POST" {
        name := req.FormValue("name")
        email := req.FormValue("email")
        phonenumber := req.FormValue("phonenumber")
        password := req.FormValue("password")
        confirmPassword := req.FormValue("confirm_password")

        if password != confirmPassword {
            http.Error(w, "Passwords do not match", http.StatusBadRequest)
            return
        }

        hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
        if err != nil {
            log.Printf("Error hashing password: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        _, err = db.Exec("INSERT INTO Users (name, phonenumber, email, password_hash, role) VALUES (?, ?, ?, ?, ?)",
            name, phonenumber, email, string(hashedPassword), "user")
        if err != nil {
            log.Printf("Error inserting user into database: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        http.Redirect(w, req, "/login", http.StatusSeeOther)
        return
    }

    http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// loginHandler handles user login
func loginHandler(w http.ResponseWriter, req *http.Request) {
    if req.Method == "GET" {
        tmpl, err := template.ParseFiles(
            "./src/templates/layout.html",
            "./src/templates/components/nav.html",
            "./src/templates/components/footer.html",
            "./src/templates/pages/login.html",
        )
        if err != nil {
            log.Printf("Error parsing login templates: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        err = tmpl.ExecuteTemplate(w, "root_template", nil)
        if err != nil {
            log.Printf("Error executing login template: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        return
    }

    if req.Method == "POST" {
        email := req.FormValue("email")
        password := req.FormValue("password")

        var id int
        var storedHashedPassword string
        var role string
        err := db.QueryRow("SELECT id, password_hash, role FROM Users WHERE email = ?", email).Scan(&id, &storedHashedPassword, &role)
        if err != nil {
            if err == sql.ErrNoRows {
                http.Error(w, "Invalid credentials", http.StatusUnauthorized)
                return
            }
            log.Printf("Error querying user from database: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        err = bcrypt.CompareHashAndPassword([]byte(storedHashedPassword), []byte(password))
        if err != nil {
            http.Error(w, "Invalid credentials", http.StatusUnauthorized)
            return
        }

        // Login successful, create session
        sessionID := generateSessionID()
        sessions[sessionID] = UserSession{UserID: id, Email: email, Role: role}
        setSessionCookie(w, sessionID)

        http.Redirect(w, req, "/", http.StatusSeeOther) // Redirect to home page
        return
    }

    http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// logoutHandler handles user logout
func logoutHandler(w http.ResponseWriter, req *http.Request) {
    cookie, err := req.Cookie("session_id")
    if err != nil {
        http.Redirect(w, req, "/login", http.StatusSeeOther) // Already logged out or no session
        return
    }

    sessionID := cookie.Value
    delete(sessions, sessionID) // Remove session from map

    // Clear the session cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "session_id",
        Value:    "",
        Path:     "/",
        Expires:  time.Unix(0, 0), // Set expiry to a past date to delete
        HttpOnly: true,
        Secure:   false,
        SameSite: http.SameSiteLaxMode,
    })

    http.Redirect(w, req, "/login", http.StatusSeeOther)
}

// requireAuth is a middleware that checks if a user is authenticated
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, req *http.Request) {
        _, ok := getSessionUser(req)
        if !ok {
            http.Redirect(w, req, "/login", http.StatusSeeOther)
            return
        }
        next.ServeHTTP(w, req)
    }
}

// requireRole is a middleware that checks if the authenticated user has one of the allowed roles
func requireRole(allowedRoles []string, next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, req *http.Request) {
        userSession, ok := getSessionUser(req)
        if !ok {
            http.Redirect(w, req, "/login", http.StatusSeeOther) // Not authenticated
            return
        }

        // Check if the user's role is in the allowedRoles list
        isAuthorized := false
        for _, role := range allowedRoles {
            if userSession.Role == role {
                isAuthorized = true
                break
            }
        }

        if !isAuthorized {
            http.Error(w, "Forbidden: You do not have the required role to access this page.", http.StatusForbidden)
            return
        }

        next.ServeHTTP(w, req)
    }
}

func main() {
        var err error
        cwd, err := os.Getwd()
        if err != nil {
                log.Fatal(err)
        }
        dbPath := filepath.Join(cwd, "db", "t.db")

        db, err = sql.Open("sqlite3", dbPath); if (err != nil) {
                log.Fatal(err)
        }

        db.QueryRow(`SELECT sqlite_version()`).Scan(&version)

        fmt.Printf("SQLLITE DB VERSION %s - Up and running", version)
        defer db.Close()

        //mux
        mux := http.NewServeMux()

	// serving static files (CSS, JS)
        staticFS := http.FileServer(http.Dir("./src/static"))
        mux.Handle("/static/", http.StripPrefix("/static/", staticFS))

        mux.HandleFunc("/signup", signupHandler)
        mux.HandleFunc("/login", loginHandler)
        mux.HandleFunc("/logout", logoutHandler)

        mux.HandleFunc("/search", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			fmt.Println("Handling the POST")
		}

                tmpl, err := template.ParseFiles(
                        "./src/templates/layout.html",
                        "./src/templates/components/nav.html", 
			"./src/templates/components/footer.html",
                        "./src/templates/pages/search.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                    // Add specific data for search page if needed
                }
                tmpl.ExecuteTemplate(w, "root_template", data)
        })

        mux.HandleFunc("/account", requireAuth(func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./src/templates/layout.html",
                        "./src/templates/components/nav.html", 
			"./src/templates/components/footer.html",
                        "./src/templates/pages/account.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                    // Add specific data for account page if needed
                }
                tmpl.ExecuteTemplate(w, "root_template", data)
        }))
         mux.HandleFunc("/cart", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./src/templates/layout.html",
                        "./src/templates/components/nav.html", 
			"./src/templates/components/footer.html",
                        "./src/templates/pages/cart.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                    // Add specific data for cart page if needed
                }
                tmpl.ExecuteTemplate(w, "root_template", data)
        })
        mux.HandleFunc("/checkout", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./src/templates/layout.html",
                        "./src/templates/components/nav.html", 
			"./src/templates/components/footer.html",
                        "./src/templates/pages/checkout.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                    // Add specific data for checkout page if needed
                }
                tmpl.ExecuteTemplate(w, "root_template", data)
        })

        mux.HandleFunc("/delivery", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./src/templates/layout.html",
                        "./src/templates/components/nav.html", 
			"./src/templates/components/footer.html",
                        "./src/templates/pages/delivery.html",
                )
                      
                if err != nil {
                        log.Fatal(err)
                }

                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                    // Add specific data for delivery page if needed
                }
                tmpl.ExecuteTemplate(w, "root_template", data)
        })
 
        mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
                tmpl, err := template.ParseFiles(
                        "./src/templates/layout.html",
                        "./src/templates/components/nav.html",
                        "./src/templates/components/footer.html",
                        "./src/templates/index.html",
                )
                if (err != nil) {
                        log.Fatal(err)
                }

                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                }

		showcase_items_query := `
		SELECT * FROM names;
		`

		rows, err := db.Query(showcase_items_query)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()
		
		var persona_array[] Persona
		for rows.Next() {
			var persona Persona
			if err := rows.Scan(&persona.Name, &persona.Phonenumber); err != nil {
				log.Fatal(err)
			}

			persona_array = append(persona_array, Persona{persona.Name, persona.Phonenumber})
		}

                data.Personas = persona_array // Assign persona_array to TemplateData

                tmpl.ExecuteTemplate(w, "root_template", data)
        })
         
        mux.HandleFunc("/partner/dashboard", requireRole([]string{"partner"}, func(w http.ResponseWriter, req *http.Request) {
            tmpl, err := template.ParseFiles(
                "./src/templates/layout.html",
                "./src/templates/components/nav.html",
                "./src/templates/components/footer.html",
                "./src/templates/pages/partner_dashboard.html",
            )
            if err != nil {
                log.Printf("Error parsing partner dashboard templates: %v", err)
                http.Error(w, "Internal Server Error", http.StatusInternalServerError)
                return
            }

            userSession, isAuthenticated := getSessionUser(req)
            data := TemplateData{
                User: userSession,
                IsAuthenticated: isAuthenticated,
            }
            tmpl.ExecuteTemplate(w, "root_template", data)
        }))

        srv := &http.Server{
                Handler: mux,
                Addr: "127.0.0.1:8000",
                WriteTimeout: 15 * time.Second,
                ReadTimeout: 15 * time.Second,
        }

        fmt.Printf("\n---\nServer is running locally at ADDR: http://%s \nCTRL-C to stop the server\n", srv.Addr)

        log.Fatal(srv.ListenAndServe())
}
