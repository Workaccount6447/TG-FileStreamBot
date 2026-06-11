package main

import (
	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/bot"
	"EverythingSuckz/fsb/internal/cache"
	"EverythingSuckz/fsb/internal/database"
	"EverythingSuckz/fsb/internal/routes"
	"EverythingSuckz/fsb/internal/types"
	"EverythingSuckz/fsb/internal/utils"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var runCmd = &cobra.Command{
	Use:                "run",
	Short:              "Run the bot with the given configuration.",
	DisableSuggestions: false,
	Run:                runApp,
}

var startTime time.Time = time.Now()

func runApp(cmd *cobra.Command, args []string) {
	// initialize logger early so config loading is logged to file
	utils.InitLogger(false)
	config.Load(utils.Logger, cmd)
	// reinitialize with correct dev mode
	utils.InitLogger(config.ValueOf.Dev)
	log := utils.Logger
	mainLogger := log.Named("Main")
	mainLogger.Info("Starting server")

	// Initialize MongoDB database (optional)
	if config.ValueOf.DatabaseURL != "" {
		mainLogger.Info("Connecting to MongoDB...")
		if err := database.Init(config.ValueOf.DatabaseURL); err != nil {
			mainLogger.Sugar().Warnf("Failed to connect to MongoDB: %v — admin features disabled", err)
		} else {
			mainLogger.Info("MongoDB connected successfully")
		}
	} else {
		mainLogger.Info("DATABASE_URL not set — admin features (ban/broadcast/status) disabled")
	}

	router := getRouter(log)

	mainBot, err := bot.StartClient(log)
	if err != nil {
		log.Sugar().Fatalf("Failed to start main bot: %v", err)
	}
	cache.InitCache(log)
	workers, err := bot.StartWorkers(log)
	if err != nil {
		log.Sugar().Fatalf("Failed to start workers: %v", err)
	}
	workers.AddDefaultClient(mainBot, mainBot.Self)
	bot.StartUserBot(log)
	mainLogger.Info("Server started", zap.Int("port", config.ValueOf.Port))
	mainLogger.Info("File Stream Bot", zap.String("version", versionString))
	mainLogger.Sugar().Infof("Server is running at %s", config.ValueOf.Host)
	err = router.Run(fmt.Sprintf(":%d", config.ValueOf.Port))
	if err != nil {
		mainLogger.Sugar().Fatalf("Server failed to start: %v", err)
	}
}

func getRouter(log *zap.Logger) *gin.Engine {
	if config.ValueOf.Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()
router.Use(gin.ErrorLogger())

router.GET("/", func(ctx *gin.Context) {
    ctx.JSON(http.StatusOK, types.RootResponse{
        Message: "Server is running.",
        Ok:      true,
        Uptime:  utils.TimeFormat(uint64(time.Since(startTime).Seconds())),
        Version: versionString,
    })
})

router.HEAD("/uptime", func(ctx *gin.Context) {
    ctx.Status(http.StatusOK)
})

routes.Load(log, router)
return router
}
