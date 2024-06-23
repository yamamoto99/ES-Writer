package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// AIに送信するリクエストの構造体
type AiRequest struct {
	Contents []Content `json:"contents"`
}

// リクエスト内のコンテンツ部分
type Content struct {
	Parts []Part `json:"parts"`
}

// コンテンツ部分の中身の文章
type Part struct {
	Text string `json:"text"`
}

// HTMLリクエスト
type HtmlRequest struct {
	Html string `json:"html"`
}

// AIからのレスポンスを受け取る
type AiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// ユーザープロフィールの構造体
type UserProfile struct {
	Bio        string
	Experience string
	Projects   string
}

// プロフィールを取得する関数
func getUserProfile(userID string) (UserProfile, error) {
	var profile UserProfile
	err := db.QueryRow("SELECT bio, experience, projects FROM users WHERE id=$1", userID).Scan(&profile.Bio, &profile.Experience, &profile.Projects)
	if err != nil {
		return profile, fmt.Errorf("プロフィールの取得に失敗しました: %v", err)
	}
	return profile, nil
}

func sendToAi(ctx context.Context, question string) (string, error) {
	// エンドポイントURLを設定
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/gemini-1.5-flash:generateContent?key=%s", apiKey)
	// 質問を含むリクエストボディをJSON形式に変換
	reqBody, err := json.Marshal(AiRequest{
		Contents: []Content{
			{
				Parts: []Part{
					{Text: question},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	// url先に質問(reqBody)を送るオブジェクト作成 ctx=リクエストのサイクルを制御する、タイムアウトやキャンセルなど
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// httpクライアントの初期化
	client := &http.Client{}
	// httpリクエスト(req)を送信
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// レスポンスの中身の読み取り
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 確認用
	// fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	// fmt.Printf("Response Body: %s\n", string(body))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error: request %d: %s", resp.StatusCode, body)
	}

	// レスポンスをjson形式からAiREsponseの構造体の型に直す
	var geminiResp AiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", err
	}

	// 中身がある(正しく返却された)時
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		return geminiResp.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no answer found")
}

func generatePromptWithBio(profile UserProfile, question string) string {
	combinedBio := fmt.Sprintf("%sです。今までの経験は%sです。これまでに作ってきた作品は%s", profile.Bio, profile.Experience, profile.Projects)
	return fmt.Sprintf("あなたの経歴は%sです。以下の質問に答えてください。簡潔かつ具体的に記述し、#や*,-などは使用せずに平文で解答部分のみを出力してください。\n%s", combinedBio, question)
}

func processQuestionsWithAI(w http.ResponseWriter, r *http.Request) {
	// CORSヘッダーを追加
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// OPTIONSリクエストに対する処理
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// トークンからユーザーIDを取得
	userID, err := getValueFromToken(r, "sub")
	if err != nil {
		http.Error(w, fmt.Sprintf("トークンからユーザーIDの取得に失敗しました: %v", err), http.StatusUnauthorized)
		return
	}

	// ユーザープロフィールを取得
	profile, err := getUserProfile(userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("ユーザープロフィールの取得に失敗しました: %v", err), http.StatusInternalServerError)
		return
	}

	// HTMLの読み込み
	var req HtmlRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("Error decoding request body: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// 不要な部分を取り除く
	cleanHtml := cleanHTMLContent(req.Html)
	log.Printf("Cleaned HTML: %s", cleanHtml)

	// 質問の抽出
	questions := extractQuestions(cleanHtml)
	if len(questions) == 0 {
		log.Printf("No questions found in the HTML content")
		http.Error(w, "No questions found", http.StatusBadRequest)
		return
	}

	// 並列処理のためのWaitGroupを作成
	var wg sync.WaitGroup

	// コンテキストを設定
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type Answer struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}

	answers := make([]Answer, len(questions))

	// 時間計測開始
	startTime := time.Now()

	// 質問ごとにゴルーチンを作成して並列処理を実行
	for i, question := range questions {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			prompt := generatePromptWithBio(profile, q)
			answer, err := sendToAi(ctx, prompt)
			if err != nil {
				log.Printf("Error sending to AI: %v", err)
				return
			}
			answers[i] = Answer{Question: q, Answer: answer}
		}(i, question)
	}

	// 全てのゴルーチンが終了するのを待機
	wg.Wait()

	// 時間計測(確認用)
	elapsedTime := time.Since(startTime)
	fmt.Printf("Total processing time: %s\n", elapsedTime)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(answers)
}

// func main() {
//  http.HandleFunc("/getAnswers", processQuestionsWithAI)
//  log.Fatal(http.ListenAndServe(":8080", nil))
// }
