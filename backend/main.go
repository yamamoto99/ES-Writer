package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type User struct {
	ID        string
	Name      string
	Email     string
	CreatedAt string
}


var db *sql.DB

var cognitoRegion string
var clientId string
var jwksURL string
var apiKey string

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handler called...")

	var user User
	err := db.QueryRow("SELECT id, username, email, created_at FROM users WHERE username = $1", "testuser").Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		fmt.Printf("error in query: %s", err)
		log.Fatal(err)
	}

	fmt.Fprintf(w, "ID: %s, Name: %s, Email: %s, CreatedAt: %s\n", user.ID, user.Name, user.Email, user.CreatedAt)
}

func main() {
	fmt.Println("server started...")
	var err error
	// dbHost := os.Getenv("DB_HOST")
	// dbUser := os.Getenv("DB_USER")
	// dbPassword := os.Getenv("DB_PASSWORD")
	// dbName := os.Getenv("DB_NAME")
	db, err = sql.Open("postgres", "host=db user=postgres password=postgres dbname=testdb sslmode=disable")
	if err != nil {
		fmt.Println("error in db connection")
		log.Fatal(err)
	}
	defer db.Close()

	err = godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	cognitoRegion = os.Getenv("COGNITO_REGION")
	clientId = os.Getenv("COGNITO_CLIENT_ID")
	jwksURL = os.Getenv("TOKEN_KEY_URL")
	apiKey = os.Getenv("GOOGLE_API_KEY")
	if cognitoRegion == "" || clientId == "" || jwksURL == "" || apiKey == "" {
		log.Fatalf("Required environment variables are not set")
	}

	http.HandleFunc("/", handler)
	http.HandleFunc("/signin", signin)
	http.HandleFunc("/signup", signup)
	http.HandleFunc("/checkEmail", checkEmail)
	http.HandleFunc("/resentEmail", resentEmail)
	http.HandleFunc("/welcome", welcome)
	http.HandleFunc("/updatesentence", updateSentence)
	http.HandleFunc("/getAnswers", processQuestionsWithAI)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
