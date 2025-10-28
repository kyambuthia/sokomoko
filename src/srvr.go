package main

import (
        "fmt"
        "log"
        "html/template"
        "net/http"
        "time"
        "path/filepath"
        "os"
        "strconv"
        "github.com/google/uuid"

        "golang.org/x/crypto/bcrypt"
        "gorm.io/driver/sqlite"
        "gorm.io/gorm"
)

type User struct {
    gorm.Model
    Name          string `json:"name"`
    Phonenumber   string `json:"phonenumber"`
    Email         string `json:"email"`
    PasswordHash  string `json:"-"`
    Role          string `json:"role"`
}

type Product struct {
    gorm.Model
    Name          string  `json:"name"`
    Price         float64 `json:"price"`
    Category      string  `json:"category"`
    Description   string  `json:"description"`
    StockQuantity int     `json:"stock_quantity"`
    PartnerID     uint    `json:"partner_id"`
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

var db *gorm.DB
var version string

// TemplateData holds data to be passed to HTML templates
type TemplateData struct {
    User UserSession
    IsAuthenticated bool
    Products []Product // Added for partner dashboard
    // TODO: Add Orders []Order for partner sales/orders view
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

        user := User{Name: name, Phonenumber: phonenumber, Email: email, PasswordHash: string(hashedPassword), Role: "user"}
        result := db.Create(&user)
        if result.Error != nil {
            log.Printf("Error inserting user into database: %v", result.Error)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        http.Redirect(w, req, "/login", http.StatusSeeOther)
        return
    }

    http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// partnerSignupHandler handles partner registration
func partnerSignupHandler(w http.ResponseWriter, req *http.Request) {
    if req.Method == "GET" {
        tmpl, err := template.ParseFiles(
            "./src/templates/layout.html",
            "./src/templates/components/nav.html",
            "./src/templates/components/footer.html",
            "./src/templates/pages/partner_signup.html",
        )
        if err != nil {
            log.Printf("Error parsing partner signup templates: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        err = tmpl.ExecuteTemplate(w, "root_template", nil)
        if err != nil {
            log.Printf("Error executing partner signup template: %v", err)
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

        user := User{Name: name, Phonenumber: phonenumber, Email: email, PasswordHash: string(hashedPassword), Role: "partner"}
        result := db.Create(&user)
        if result.Error != nil {
            log.Printf("Error inserting partner into database: %v", result.Error)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        http.Redirect(w, req, "/login", http.StatusSeeOther) // Redirect to login page for partners
        return
    }

    http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// addProductHandler handles adding new products by partners
func addProductHandler(w http.ResponseWriter, req *http.Request) {
    if req.Method == "GET" {
        tmpl, err := template.ParseFiles(
            "./src/templates/layout.html",
            "./src/templates/components/nav.html",
            "./src/templates/components/footer.html",
            "./src/templates/pages/add_product.html",
        )
        if err != nil {
            log.Printf("Error parsing add product templates: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        userSession, isAuthenticated := getSessionUser(req)
        data := TemplateData{
            User: userSession,
            IsAuthenticated: isAuthenticated,
        }
        err = tmpl.ExecuteTemplate(w, "root_template", data)
        if err != nil {
            log.Printf("Error executing add product template: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        return
    }

    if req.Method == "POST" {
        userSession, ok := getSessionUser(req)
        if !ok || userSession.Role != "partner" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }

        name := req.FormValue("name")
        description := req.FormValue("description")
        priceStr := req.FormValue("price")
        category := req.FormValue("category")
        stockQuantityStr := req.FormValue("stock_quantity")

        price, err := strconv.ParseFloat(priceStr, 64)
        if err != nil {
            http.Error(w, "Invalid price format", http.StatusBadRequest)
            return
        }
        stockQuantity, err := strconv.Atoi(stockQuantityStr)
        if err != nil {
            http.Error(w, "Invalid stock quantity format", http.StatusBadRequest)
            return
        }

        product := Product{Name: name, Description: description, Price: price, Category: category, StockQuantity: stockQuantity, PartnerID: uint(userSession.UserID)}
        result := db.Create(&product)
        if result.Error != nil {
            log.Printf("Error inserting product into database: %v", result.Error)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        http.Redirect(w, req, "/partner/dashboard", http.StatusSeeOther)
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

        var user User
        result := db.Where("email = ?", email).First(&user)
        if result.Error != nil {
            if result.Error == gorm.ErrRecordNotFound {
                http.Error(w, "Invalid credentials", http.StatusUnauthorized)
                return
            }
            log.Printf("Error querying user from database: %v", result.Error)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
        if err != nil {
            http.Error(w, "Invalid credentials", http.StatusUnauthorized)
            return
        }

        // Login successful, create session
        sessionID := generateSessionID()
        sessions[sessionID] = UserSession{UserID: int(user.ID), Email: user.Email, Role: user.Role}
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

        db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
        if err != nil {
                log.Fatal("failed to connect database")
        }

        // Drop tables to ensure a clean slate on each run (for development)
        db.Migrator().DropTable(&User{}, &Product{})

        // Migrate the schema
        db.AutoMigrate(&User{}, &Product{})

        // Insert sample products
        db.Create(&Product{Name: "Laptop", Price: 1200, Category: "Electronics", Description: "A powerful laptop for all your computing needs.", StockQuantity: 10, PartnerID: 1})
        db.Create(&Product{Name: "Smartphone", Price: 800, Category: "Electronics", Description: "The latest smartphone with a stunning display.", StockQuantity: 25, PartnerID: 1})
        db.Create(&Product{Name: "Coffee Maker", Price: 100, Category: "Home Appliances", Description: "Brew the perfect cup of coffee every morning.", StockQuantity: 50, PartnerID: 1})

        fmt.Printf("SQLLITE DB Up and running")

        // Main application mux
        mainMux := http.NewServeMux()



        // serving static files (CSS, JS) for main application
        staticFS := http.FileServer(http.Dir("./src/static"))
        mainMux.Handle("/static/", http.StripPrefix("/static/", staticFS))

        mainMux.HandleFunc("/signup", signupHandler)
        mainMux.HandleFunc("/login", loginHandler)
        mainMux.HandleFunc("/logout", logoutHandler)

        // Partner routes
        mainMux.HandleFunc("/partner/signup", partnerSignupHandler)
        mainMux.HandleFunc("/partner/products/add", requireRole([]string{"partner"}, addProductHandler))



        mainMux.HandleFunc("/search", func(w http.ResponseWriter, req *http.Request) {
                userSession, isAuthenticated := getSessionUser(req)
                data := TemplateData{
                    User: userSession,
                    IsAuthenticated: isAuthenticated,
                }

		if req.Method == "POST" {
			searchQuery := req.FormValue("search_query")
                        var products []Product
                        db.Where("name LIKE ? OR description LIKE ?", "%"+searchQuery+"%", "%"+searchQuery+"%").Find(&products)
                        data.Products = products
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

                tmpl.ExecuteTemplate(w, "root_template", data)
        })

        mainMux.HandleFunc("/account", requireAuth(func(w http.ResponseWriter, req *http.Request) {
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
        mainMux.HandleFunc("/cart", func(w http.ResponseWriter, req *http.Request) {
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
        mainMux.HandleFunc("/checkout", func(w http.ResponseWriter, req *http.Request) {
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

        mainMux.HandleFunc("/delivery", func(w http.ResponseWriter, req *http.Request) {
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
        mainMux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
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

		var products []Product
		db.Find(&products)

                data.Products = products // Assign products to TemplateData

                tmpl.ExecuteTemplate(w, "root_template", data)
        })
         
        mainMux.HandleFunc("/partner/dashboard", requireRole([]string{"partner"}, func(w http.ResponseWriter, req *http.Request) {
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

            var products []Product
            result := db.Where("partner_id = ?", userSession.UserID).Find(&products)
            if result.Error != nil {
                log.Printf("Error querying products for partner %d: %v", userSession.UserID, result.Error)
                http.Error(w, "Internal Server Error", http.StatusInternalServerError)
                return
            }
            data.Products = products // Assign products to TemplateData

            tmpl.ExecuteTemplate(w, "root_template", data)
        }))

        srv := &http.Server{
                Handler: mainMux,
                Addr: "127.0.0.1:8000",
                WriteTimeout: 15 * time.Second,
                ReadTimeout: 15 * time.Second,
        }

        fmt.Printf("\n---\nServer is running locally at ADDR: http://%s \nCTRL-C to stop the server\n", srv.Addr)

        log.Fatal(srv.ListenAndServe())
}
