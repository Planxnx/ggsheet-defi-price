package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-co-op/gocron"
	"github.com/planxnx/ggsheet-defi-price/arken"
	"golang.org/x/oauth2/google"
	"gopkg.in/Iwark/spreadsheet.v2"
)

var google_application_creadential []byte

func init() {
	TZ := defaultValue(os.Getenv("TZ"), "Asia/Bangkok")
	loc, err := time.LoadLocation(TZ)
	if err != nil {
		log.Printf("[ERROR] failed to load timezone, %+v", err)
		os.Exit(1)
	}
	time.Local = loc
}

func init() {
	CREDENTIALS := os.Getenv("CREDENTIALS")
	if CREDENTIALS == "" {
		log.Println("CREDENTIALS is not set")
		os.Exit(1)
	}

	var creadential struct {
		Type                    string `json:"type"`
		ProjectID               string `json:"project_id"`
		PrivateKeyID            string `json:"private_key_id"`
		PrivateKey              string `json:"private_key"`
		ClientEmail             string `json:"client_email"`
		ClientID                string `json:"client_id"`
		AuthURI                 string `json:"auth_uri"`
		TokenURI                string `json:"token_uri"`
		AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
		ClientX509CertURL       string `json:"client_x509_cert_url"`
	}
	if err := json.Unmarshal([]byte(CREDENTIALS), &creadential); err != nil {
		log.Printf("[ERROR] CREDENTIALS is not valid json, %+v\n", err)
		os.Exit(1)
	}

	data, err := json.Marshal(creadential)
	if err != nil {
		log.Printf("[ERROR] CREDENTIALS is not valid json, %+v\n", err)
		os.Exit(1)
	}

	google_application_creadential = data
}

func main() {
	apiURL := os.Getenv("API_URL")
	apiUsername := os.Getenv("API_USERNAME")
	apiToken := os.Getenv("API_TOKEN")
	cronExp := defaultValue(os.Getenv("CRON_EXP"), "0 */12 * * *")
	spreadsheetID := os.Getenv("SPREADSHEET_ID")
	sheetID := defaultValue(os.Getenv("SHEET_ID_PRICES"), "1493661491")

	// Context with gracefully shutdown signal
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGHUP,  // kill -SIGHUP XXXX
		syscall.SIGINT,  // kill -SIGINT XXXX or Ctrl+c
		syscall.SIGQUIT, // kill -SIGQUIT XXXX
		syscall.SIGTERM, // kill -SIGTERM XXXX
	)
	defer stop()

	// Load credentials
	conf, err := google.JWTConfigFromJSON(google_application_creadential, spreadsheet.Scope)
	if err != nil {
		log.Printf("[ERROR] CREDENTIALS is not valid, %+v\n", err)
		os.Exit(1)
	}

	// Create a new Spreadsheet Service
	service := spreadsheet.NewServiceWithClient(conf.Client(ctx))

	// Create a new Arken Public API Client
	arkenAPI := arken.New(apiURL, apiUsername, apiToken)

	s := gocron.NewScheduler(time.UTC)
	if _, err := s.Cron(cronExp).StartImmediately().Do(func() {
		// Fetch spreadsheet of Defi Portfolio
		spreadsheet, err := service.FetchSpreadsheet(spreadsheetID)
		if err != nil {
			log.Printf("[ERROR] failed to fetch spreadsheet, %+v", err)
			os.Exit(1)
		}

		log.Printf("[START] Processeing spreadsheet: %s, Time: %s\n", spreadsheet.Properties.Title, time.Now().Local())

		sheetIDPrices, _ := strconv.ParseUint(sheetID, 10, 64)
		if sheetIDPrices == 0 {
			log.Println("Invalid Prices Sheet ID")
			os.Exit(1)
		}

		// Update total assets
		pricesSheet, err := spreadsheet.SheetByID(uint(sheetIDPrices))
		if err != nil {
			log.Printf("[ERROR] failed to fetch total assets sheet, %+v", err)
			os.Exit(1)
		}

		var (
			totalTokens float64
			now         = time.Now().Local()
		)

		currentRow := len(pricesSheet.Columns[0])
		for i, row := range pricesSheet.Columns[0] {
			if strings.Trim(row.Value, " ") == "" {
				currentRow = i
				break
			}
		}

		// Gracefully shutdown
		defer func() {
			if err := pricesSheet.Synchronize(); err != nil {
				log.Printf("[ERROR] failed to sync prices sheet, %+v", err)
				os.Exit(1)
			}
			log.Printf("[DONE] Processeing spreadsheet: %s at Row %v, %v Tokens, Durations: %v\n", spreadsheet.Properties.Title, currentRow+1, totalTokens, time.Since(now))
		}()

		// Add date
		pricesSheet.Update(currentRow, 0, fmt.Sprintf("%.2f", spreadSheetDate(now)))

		for i := 1; i < len(pricesSheet.Columns); i++ {
			cols := pricesSheet.Columns[i]
			if len(cols) <= 3 {
				// required column: Name, ChainID, Address
				continue
			}
			if !common.IsHexAddress(cols[2].Value) {
				// invalid address
				continue
			}
			name := cols[0].Value
			chainID := cols[1].Value
			address := common.HexToAddress(cols[2].Value)

			log.Printf("Processing %s %s %s\n", name, chainID, address.Hex())

			price, err := arkenAPI.GetPrice(ctx, chainID, address)
			if err != nil {
				log.Fatalf("[ERROR] failed to get latest price, %+v", err)
			}

			oldPrice := cols[currentRow-1].EffectiveValue().NumberValue
			change := 1.0
			if oldPrice != 0 {
				change = (price - oldPrice) / oldPrice
			}

			pricesSheet.Update(currentRow, i, fmt.Sprintf("%f", price))
			pricesSheet.Update(currentRow, i+1, fmt.Sprintf("%.4f", change))
			totalTokens++
		}
	}); err != nil {
		log.Printf("[ERROR] failed to create scheduler, %+v", err)
		os.Exit(1)
	}

	s.StartAsync()
	log.Println("[INFO] Start scheduler")
	<-ctx.Done()
	s.Stop()
	log.Println("[INFO] Stop scheduler")
}

var zeroSpreadSheetTime = Must(time.Parse(time.RFC3339, "1899-12-30T00:00:00+07:00"))

func spreadSheetDate(t ...time.Time) float64 {
	if len(t) == 0 {
		t = append(t, time.Now().Local())
	}

	return math.Floor(t[0].Sub(zeroSpreadSheetTime).Hours() / 24)
}

func defaultValue[T comparable](value T, defaultValue ...T) T {
	var zero T
	if value == zero && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

func Must[T any](val T, err any) T {
	switch e := err.(type) {
	case bool:
		if !e {
			panic("not ok")
		}
	case error:
		if e != nil {
			panic(e)
		}
	default:
		if err != nil {
			panic(fmt.Sprintf("invalid error type, must be bool or error, got %T(%v)", err, err))
		}
	}
	return val
}
