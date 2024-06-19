package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/golang-jwt/jwt/v5"
)

// JWTのクレームの構造体を定義
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// リクエストボディの構造体を定義
var SignUp struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

var SignIn struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var CheckEmail struct {
	Username string `json:"username"`
	VerificationCode string `json:"verificationCode"`
}

type SentenceUpdateInput struct {
	UserID   string `json:"userid"`
	Sentence string `json:"sentence"`
}

func createCognitoClient(ctx context.Context) (*cognitoidentityprovider.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cognitoRegion))
	if err != nil {
		return nil, err
	}
	svc := cognitoidentityprovider.NewFromConfig(cfg)
	return svc, nil
}

func checkEmail(w http.ResponseWriter, r *http.Request) {
	// POSTメソッド以外は許可しない
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	err := json.NewDecoder(r.Body).Decode(&CheckEmail)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	
	// 必須フィールドが空でないか確認
	if CheckEmail.VerificationCode == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// クッキーからusernameを取得
	c, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Error(w, fmt.Sprintf("%vnot found: %v", "username", err), http.StatusBadRequest)
			return
		}
		http.Error(w, fmt.Sprintf("Invalid request: Cookie retrieval error: %v", err), http.StatusBadRequest)
		return
	}

	// AWS設定の読み込み
	svc, err := createCognitoClient(r.Context())
	if err != nil {
		http.Error(w, "Unable to load SDK config", http.StatusInternalServerError)
		return
	}

	// 確認コードの入力を設定
	confirmSignUpInput := &cognitoidentityprovider.ConfirmSignUpInput{
		ClientId:         aws.String(clientId),
		Username:         aws.String(c.Value),
		ConfirmationCode: aws.String(CheckEmail.VerificationCode),
	}

	// Cognitoに確認コードを送信
	_, err = svc.ConfirmSignUp(r.Context(), confirmSignUpInput)
	if err != nil {
		http.Error(w, "Confirmation failed", http.StatusInternalServerError)
		log.Println("Confirmation error:", err)
		return
	}

	// 確認成功のレスポンスを返す
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User confirmed successfully")
}

func resentEmail(w http.ResponseWriter, r *http.Request) {
	// POSTメソッド以外は許可しない
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// クッキーからusernameを取得
	c, err := r.Cookie("username")
	if err != nil {
		if err == http.ErrNoCookie {
			http.Error(w, fmt.Sprintf("%vnot found: %v", "username", err), http.StatusBadRequest)
			return
		}
		http.Error(w, fmt.Sprintf("Invalid request: Cookie retrieval error: %v", err), http.StatusBadRequest)
		return
	}

	// AWS設定の読み込み
	svc, err := createCognitoClient(r.Context())
	if err != nil {
		http.Error(w, "Unable to load SDK config", http.StatusInternalServerError)
		return
	}

	// 再送リクエストの作成
	input := &cognitoidentityprovider.ResendConfirmationCodeInput{
		ClientId: aws.String(clientId),
		Username: aws.String(c.Value),
	}

	// 確認メール再送の実行
	_, err = svc.ResendConfirmationCode(context.TODO(), input)
	if err != nil {
		http.Error(w, fmt.Sprintf("Confirmation email resend error: %v", err), http.StatusInternalServerError)
		return
	}

	// 成功メッセージの送信
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("The confirmation email has been resent."))
}

func signup(w http.ResponseWriter, r *http.Request) {
	// POSTメソッド以外は許可しない
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// クライアントからのリクエストボディをデコードして、ユーザー情報を取得
	err := json.NewDecoder(r.Body).Decode(&SignUp)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// 必須フィールドが空でないか確認
	if SignUp.Username == "" || SignUp.Password == "" || SignUp.Email == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// AWS設定の読み込み
	svc, err := createCognitoClient(r.Context())
	if err != nil {
		http.Error(w, "Unable to load SDK config", http.StatusInternalServerError)
		return
	}

	// サインアップリクエストの入力を設定
	signUpInput := &cognitoidentityprovider.SignUpInput{
		ClientId: aws.String(clientId),
		Username: aws.String(SignUp.Username),
		Password: aws.String(SignUp.Password),
		UserAttributes: []types.AttributeType{
			{
				Name:  aws.String("email"),
				Value: aws.String(SignUp.Email),
			},
		},
	}

	// Cognitoにサインアップリクエストを送信
	signUpOutput, err := svc.SignUp(r.Context(), signUpInput)
	if err != nil {
		http.Error(w, "Signup failed", http.StatusInternalServerError)
		log.Println("Signup error:", err)
		return
	}

	// ユーザーIDを取得
	userID := *signUpOutput.UserSub
	createdAt := time.Now()

	// サインアップ成功後にユーザー情報をデータベースに挿入
	stmt, err := db.Prepare("INSERT INTO users (id, username, email, created_at) VALUES ($1, $2, $3, $4)")
	if err != nil {
		log.Println("Database prepare statement error:", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(userID, SignUp.Username, SignUp.Email, createdAt)
	if err != nil {
		http.Error(w, "Database insert failed", http.StatusInternalServerError)
		log.Println("Database insert error:", err)
		return
	}

	// ユーザー名をCookieに保存
	http.SetCookie(w, &http.Cookie{
		Name:     "username",
		Value:    SignUp.Username,
		Expires:  time.Now().Add(10 * time.Minute),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// サインアップ成功のレスポンスを返す
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "User signed up successfully")
}

func signin(w http.ResponseWriter, r *http.Request) {
	// POSTメソッド以外は許可しない
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// クライアントからのリクエストボディをデコードして、認証情報を取得
	err := json.NewDecoder(r.Body).Decode(&SignIn)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// AWS設定の読み込み
	svc, err := createCognitoClient(r.Context())
	if err != nil {
		http.Error(w, "Unable to load SDK config", http.StatusInternalServerError)
		return
	}

	// 認証入力パラメータを設定
	authInput := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow: types.AuthFlowTypeUserPasswordAuth,
		ClientId: aws.String(clientId),
		AuthParameters: map[string]string{
			"USERNAME": SignIn.Username,
			"PASSWORD": SignIn.Password,
		},
	}

	// Cognitoにサインインリクエストを送信
	authResp, err := svc.InitiateAuth(r.Context(), authInput)
	if err != nil {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		log.Println("Authentication error:", err)
		return
	}

	// JWTトークンの取得
	idToken := authResp.AuthenticationResult.IdToken
	accessToken := authResp.AuthenticationResult.AccessToken
	refreshToken := authResp.AuthenticationResult.RefreshToken

	// トークンをクッキーに保存
	http.SetCookie(w, &http.Cookie{
		Name:     "idToken",
		Value:    *idToken,
		Expires:  time.Now().Add(1 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "accessToken",
		Value:    *accessToken,
		Expires:  time.Now().Add(1 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refreshToken",
		Value:    *refreshToken,
		Expires:  time.Now().Add(24 * 7 * time.Hour),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// サインイン成功のレスポンスを返す
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "User signed in successfully")
}

func updateSentence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var input SentenceUpdateInput
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// if SentenceUpdateInput.UserID == "" || SentenceUpdateInput.Sentence == "" {
	// 	http.Error(w, "Missing user_id or sentence", http.StatusBadRequest)
	// 	return
	// }

	stmt, err := db.Prepare("UPDATE users SET sentence = $1 WHERE id = $2")
	if err != nil {
		log.Println("Database prepare statement error:", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(input.Sentence, input.UserID)
	if err != nil {
		http.Error(w, "Database update failed", http.StatusInternalServerError)
		log.Println("Database update error:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Sentence updated successfully"))
}


