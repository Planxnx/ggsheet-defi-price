package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata"

	"github.com/gaze-network/indexer-network/pkg/logger"
	"github.com/gaze-network/indexer-network/pkg/logger/slogx"
	"github.com/go-co-op/gocron"
	"github.com/planxnx/ggsheet-defi-price/coingecko"
	"golang.org/x/oauth2/google"
	"gopkg.in/Iwark/spreadsheet.v2"
)

var google_application_creadential []byte

func init() {
	if err := logger.Init(logger.Config{
		Output: "JSON",
		Debug:  false,
	}); err != nil {
		logger.Error("Failed to initialize logger: %v", slogx.Error(err))
		panic(err)
	}
}

func init() {
	TZ := defaultValue(os.Getenv("TZ"), "Asia/Bangkok")
	loc, err := time.LoadLocation(TZ)
	if err != nil {
		logger.Error("Failed to load timezone", slogx.Error(err))
		panic(err)
	}
	time.Local = loc
}

func init() {
	CREDENTIALS := os.Getenv("CREDENTIALS")
	if CREDENTIALS == "" {
		logger.Error("CREDENTIALS is not set")
		panic("CREDENTIALS is not set")
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
		logger.Error("CREDENTIALS is not valid json", slogx.Error(err))
		panic(err)
	}

	data, err := json.Marshal(creadential)
	if err != nil {
		logger.Error("CREDENTIALS is not valid json", slogx.Error(err))
		panic(err)
	}

	google_application_creadential = data
}

func main() {
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
		logger.ErrorContext(ctx, "CREDENTIALS is not valid", slogx.Error(err))
		panic(err)
	}

	// Create a new Spreadsheet Service
	service := spreadsheet.NewServiceWithClient(conf.Client(ctx))

	// Create a new Arken Public API Client
	coingeckoAPI := coingecko.NewClient(apiToken)

	s := gocron.NewScheduler(time.UTC)
	if _, err := s.Cron(cronExp).StartImmediately().Do(func() {
		// Fetch spreadsheet of Defi Portfolio
		spreadsheet, err := service.FetchSpreadsheet(spreadsheetID)
		if err != nil {
			logger.ErrorContext(ctx, "Fetch spreadsheet failed", slogx.Error(err))
			panic(err)
		}

		ctx = logger.WithContext(ctx,
			slogx.String("title", spreadsheet.Properties.Title),
		)

		logger.InfoContext(ctx, "[START] Processeing spreadsheet", slogx.Time("time", time.Now().Local()))

		sheetIDPrices, _ := strconv.ParseUint(sheetID, 10, 64)
		if sheetIDPrices == 0 {
			logger.ErrorContext(ctx, "Invalid Prices Sheet ID")
			panic("Invalid Prices Sheet ID")
		}

		// Update total assets
		pricesSheet, err := spreadsheet.SheetByID(uint(sheetIDPrices))
		if err != nil {
			logger.ErrorContext(ctx, "Fetch total assets sheet failed", slogx.Error(err))
			panic(err)
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
				logger.ErrorContext(ctx, "Sync prices sheet failed", slogx.Error(err))
				panic(err)
			}
			logger.InfoContext(ctx, "[DONE] Processeing spreadsheet", slogx.Int("row", currentRow+1), slogx.Float64("tokens", totalTokens), slogx.Duration("duration", time.Since(now)), slogx.Stringer("durationStr", time.Since(now)))
		}()

		// Add date
		pricesSheet.Update(currentRow, 0, fmt.Sprintf("%.2f", spreadSheetDate(now)))

		for i := 1; i < len(pricesSheet.Columns); i++ {
			cols := pricesSheet.Columns[i]
			if len(cols) <= 3 {
				// required column: Name, ChainID, Address
				continue
			}
			if strings.TrimSpace(cols[1].Value) == "" {
				// invalid name
				continue
			}
			if strings.TrimSpace(cols[2].Value) == "" {
				// invalid address
				continue
			}
			name := cols[0].Value
			chainID := cols[1].Value
			address := cols[2].Value

			ctx := logger.WithContext(ctx,
				slogx.String("name", name),
				slogx.String("chainId", chainID),
				slogx.String("address", address),
			)

			logger.InfoContext(ctx, "[Processing] Fetching token price")

			if !coingeckoAPI.IsSupportedChainId(chainID) {
				logger.ErrorContext(ctx, "Unsupported chain id for CoinGecko", slogx.String("chainId", chainID))
				continue
			}

			price, err := coingeckoAPI.GetPrice(ctx, chainID, address)
			if err != nil {
				logger.ErrorContext(ctx, "Failed to get latest price", slogx.Error(err))
				panic(err)
			}

			oldPrice := cols[currentRow-1].EffectiveValue().NumberValue
			change := 1.0
			if oldPrice != 0 {
				change = (price - oldPrice) / oldPrice
			}

			pricesSheet.Update(currentRow, i, fmt.Sprintf("%f", price))
			pricesSheet.Update(currentRow, i+1, fmt.Sprintf("%.4f", change))
			totalTokens++

			logger.InfoContext(ctx, "[Processed] Fetched token price",
				slogx.Float64("price", price),
				slogx.Float64("change", change),
				slogx.Int("row", currentRow),
				slogx.Int("col", i),
			)
		}
	}); err != nil {
		logger.ErrorContext(ctx, "Failed to create cronjob scheduler", slogx.Error(err))
		panic(err)
	}

	s.StartAsync()
	logger.InfoContext(ctx, "Start scheduler")
	<-ctx.Done()
	s.Stop()
	logger.InfoContext(ctx, "Stop scheduler")
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
