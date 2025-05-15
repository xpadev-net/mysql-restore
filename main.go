package main

import (
	"bufio"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/rivo/tview"
)

const (
	maxRetries    = 5
	retryInterval = 5 * time.Second
)

func main() {
	host := flag.String("host", "localhost", "MySQL host")
	port := flag.String("port", "3306", "MySQL port")
	user := flag.String("user", "root", "MySQL user")
	password := flag.String("password", "", "MySQL password (省略可)")
	dbname := flag.String("db", "", "Database name (省略可)")
	file := flag.String("file", "", "SQL file path")
	resumeLine := flag.Int("resume-line", 1, "再開する行番号 (1始まり)")
	flag.Parse()

	if *file == "" {
		fmt.Println("Usage: mysql-restore --host <host> --port <port> --user <user> [--password <password>] [--db <dbname>] --file <file.sql> [--resume-line <n>]")
		os.Exit(1)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?multiStatements=true&parseTime=true", *user, *password, *host, *port, *dbname)
	if *dbname == "" {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/?multiStatements=true&parseTime=true", *user, *password, *host, *port)
	}

	var db *sql.DB
	var err error
	for i := 0; i < maxRetries; i++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			break
		}
		fmt.Printf("DB接続失敗: %v。%d秒後にリトライ...\n", err, int(retryInterval.Seconds()))
		time.Sleep(retryInterval)
	}
	if err != nil {
		fmt.Printf("DB接続に失敗しました: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	app := tview.NewApplication()
	progressView := tview.NewTextView().SetDynamicColors(true)
	logView := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(progressView, 3, 1, false).
		AddItem(logView, 0, 4, false)

	paused := false
	go func() {
		err := restoreSQLTUI(db, *file, *resumeLine, progressView, logView, app, &paused)
		if err != nil {
			app.QueueUpdateDraw(func() {
				current := logView.GetText(false)
				timestamp := time.Now().Format("15:04:05")
				logView.SetText(current + fmt.Sprintf("[%s][red]リストア中にエラー: %v[-]\n", timestamp, err))
				logView.ScrollToEnd()
			})
		}
		app.QueueUpdateDraw(func() {
			current := logView.GetText(false)
			timestamp := time.Now().Format("15:04:05")
			logView.SetText(current + fmt.Sprintf("[%s][green]リストア完了[-]\n", timestamp))
			logView.ScrollToEnd()
		})
		time.Sleep(1 * time.Second)
		app.Stop()
	}()

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'p':
			timestamp := time.Now().Format("15:04:05")
			paused = true
			current := logView.GetText(false)
			logView.SetText(current + fmt.Sprintf("[%s][yellow][PAUSE] 一時停止しました。'r'で再開[-]\n", timestamp))
			logView.ScrollToEnd()
			return nil
		case 'r':
			if paused {
				timestamp := time.Now().Format("15:04:05")
				paused = false
				current := logView.GetText(false)
				logView.SetText(current + fmt.Sprintf("[%s][green][RESUME] 再開します[-]\n", timestamp))
				logView.ScrollToEnd()
			}
			return nil
		}
		return event
	})

	if err := app.SetRoot(flex, true).Run(); err != nil {
		panic(err)
	}
}

func drawProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	bar := strings.Repeat("■", filled) + strings.Repeat("□", width-filled)
	return fmt.Sprintf("[%s] %.2f%%", bar, percent)
}

func restoreSQLTUI(db *sql.DB, filePath string, resumeLine int, progressView, logView *tview.TextView, app *tview.Application, paused *bool) error {
	f, err := os.Open(filePath)
	if (err != nil) {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var sb strings.Builder
	stmtCount := 0

	fileInfo, err := f.Stat()
	if err != nil {
		return err
	}
	totalSize := fileInfo.Size()
	var processedSize int64 = 0

	currentLine := 1
	for {
		if *paused {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		line, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		processedSize += int64(len(line))
		if currentLine >= resumeLine {
			sb.WriteString(line)
			if strings.HasSuffix(strings.TrimSpace(line), ";") {
				stmt := sb.String()
				stmtCount++
				percent := float64(processedSize) / float64(totalSize) * 100
				app.QueueUpdateDraw(func() {
					progressView.Clear()
					progressView.SetText(drawProgressBar(percent, 40))
					current := logView.GetText(false)
					timestamp := time.Now().Format("15:04:05")
					logView.SetText(current + fmt.Sprintf("[%s][行:%d] 実行中... (進捗: %.2f%%)\n", timestamp, currentLine, percent))
					logView.ScrollToEnd()
				})
				err := execWithRetry(db, stmt)
				if err != nil {
					app.QueueUpdateDraw(func() {
						current := logView.GetText(false)
						timestamp := time.Now().Format("15:04:05")
						logView.SetText(current + fmt.Sprintf("[%s][行:%d] 失敗\n", timestamp, currentLine))
						logView.ScrollToEnd()
					})
					return fmt.Errorf("SQL実行エラー: %w\nSQL: %s", err, stmt)
				}
				app.QueueUpdateDraw(func() {
					current := logView.GetText(false)
					timestamp := time.Now().Format("15:04:05")
					logView.SetText(current + fmt.Sprintf("[%s][行:%d] 完了\n", timestamp, currentLine))
					logView.ScrollToEnd()
				})
				sb.Reset()
			}
		}
		currentLine++
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return nil
}

func execWithRetry(db *sql.DB, stmt string) error {
	for i := 0; i < maxRetries; i++ {
		_, err := db.Exec(stmt)
		if err == nil {
			return nil
		}
		fmt.Printf("SQL実行失敗: %v。%d秒後にリトライ...\n", err, int(retryInterval.Seconds()))
		time.Sleep(retryInterval)
	}
	return fmt.Errorf("リトライ上限に到達")
}
