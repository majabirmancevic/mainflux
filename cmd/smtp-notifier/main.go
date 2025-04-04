// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/MainfluxLabs/mainflux"
	"github.com/MainfluxLabs/mainflux/consumers"
	"github.com/MainfluxLabs/mainflux/consumers/notifiers"
	"github.com/MainfluxLabs/mainflux/consumers/notifiers/api"
	httpapi "github.com/MainfluxLabs/mainflux/consumers/notifiers/api/http"
	"github.com/MainfluxLabs/mainflux/consumers/notifiers/postgres"
	"github.com/MainfluxLabs/mainflux/consumers/notifiers/smtp"
	"github.com/MainfluxLabs/mainflux/consumers/notifiers/tracing"
	"github.com/MainfluxLabs/mainflux/logger"
	"github.com/MainfluxLabs/mainflux/pkg/clients"
	clientsgrpc "github.com/MainfluxLabs/mainflux/pkg/clients/grpc"
	"github.com/MainfluxLabs/mainflux/pkg/dbutil"
	"github.com/MainfluxLabs/mainflux/pkg/email"
	"github.com/MainfluxLabs/mainflux/pkg/errors"
	"github.com/MainfluxLabs/mainflux/pkg/jaeger"
	"github.com/MainfluxLabs/mainflux/pkg/messaging/brokers"
	protomfx "github.com/MainfluxLabs/mainflux/pkg/proto"
	"github.com/MainfluxLabs/mainflux/pkg/servers"
	servershttp "github.com/MainfluxLabs/mainflux/pkg/servers/http"
	"github.com/MainfluxLabs/mainflux/pkg/uuid"
	thingsapi "github.com/MainfluxLabs/mainflux/things/api/grpc"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	"github.com/jmoiron/sqlx"
	opentracing "github.com/opentracing/opentracing-go"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
)

const (
	svcName              = "smtp-notifier"
	stopWaitTime         = 5 * time.Second
	defLogLevel          = "error"
	defFrom              = ""
	defJaegerURL         = ""
	defBrokerURL         = "nats://localhost:4222"
	defDBHost            = "localhost"
	defDBPort            = "5432"
	defDBUser            = "mainflux"
	defDBPass            = "mainflux"
	defDB                = "smtp-notifiers"
	defDBSSLMode         = "disable"
	defDBSSLCert         = ""
	defDBSSLKey          = ""
	defDBSSLRootCert     = ""
	defThingsTLS         = "false"
	defThingsCACerts     = ""
	defHTTPPort          = "9023"
	defServerCert        = ""
	defServerKey         = ""
	defThingsGRPCURL     = "localhost:8183"
	defThingsGRPCTimeout = "1s"

	defEmailHost        = "localhost"
	defEmailPort        = "25"
	defEmailUsername    = "root"
	defEmailPassword    = ""
	defEmailFromAddress = ""
	defEmailFromName    = ""
	defEmailTemplate    = "email.tmpl"

	envLogLevel          = "MF_SMTP_NOTIFIER_LOG_LEVEL"
	envFrom              = "MF_SMTP_NOTIFIER_FROM_ADDR"
	envJaegerURL         = "MF_JAEGER_URL"
	envBrokerURL         = "MF_BROKER_URL"
	envDBHost            = "MF_SMTP_NOTIFIER_DB_HOST"
	envDBPort            = "MF_SMTP_NOTIFIER_DB_PORT"
	envDBUser            = "MF_SMTP_NOTIFIER_DB_USER"
	envDBPass            = "MF_SMTP_NOTIFIER_DB_PASS"
	envDB                = "MF_SMTP_NOTIFIER_DB"
	envDBSSLMode         = "MF_SMTP_NOTIFIER_DB_SSL_MODE"
	envDBSSLCert         = "MF_SMTP_NOTIFIER_DB_SSL_CERT"
	envDBSSLKey          = "MF_SMTP_NOTIFIER_DB_SSL_KEY"
	envDBSSLRootCert     = "MF_SMTP_NOTIFIER_DB_SSL_ROOT_CERT"
	envThingsTLS         = "MF_SMTP_NOTIFIER_THINGS_TLS"
	envThingsCACerts     = "MF_SMTP_NOTIFIER_THINGS_CA_CERTS"
	envHTTPPort          = "MF_SMTP_NOTIFIER_PORT"
	envServerCert        = "MF_SMTP_NOTIFIER_SERVER_CERT"
	envServerKey         = "MF_SMTP_NOTIFIER_SERVER_KEY"
	envThingsGRPCURL     = "MF_THINGS_AUTH_GRPC_URL"
	envThingsGRPCTimeout = "MF_THINGS_AUTH_GRPC_TIMEOUT"

	envEmailHost        = "MF_EMAIL_HOST"
	envEmailPort        = "MF_EMAIL_PORT"
	envEmailUsername    = "MF_EMAIL_USERNAME"
	envEmailPassword    = "MF_EMAIL_PASSWORD"
	envEmailFromAddress = "MF_EMAIL_FROM_ADDRESS"
	envEmailFromName    = "MF_EMAIL_FROM_NAME"
	envEmailTemplate    = "MF_SMTP_NOTIFIER_TEMPLATE"
)

type config struct {
	brokerURL         string
	logLevel          string
	dbConfig          postgres.Config
	httpConfig        servers.Config
	thingsConfig      clients.Config
	emailConf         email.Config
	from              string
	jaegerURL         string
	thingsGRPCTimeout time.Duration
}

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	logger, err := logger.New(os.Stdout, cfg.logLevel)
	if err != nil {
		log.Fatalf(err.Error())
	}

	pubSub, err := brokers.NewPubSub(cfg.brokerURL, "", logger)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to connect to message broker: %s", err))
		os.Exit(1)
	}
	defer pubSub.Close()

	notifiersTracer, notifiersCloser := jaeger.Init(svcName, cfg.jaegerURL, logger)
	defer notifiersCloser.Close()

	db := connectToDB(cfg.dbConfig, logger)
	defer db.Close()

	thingsTracer, thingsCloser := jaeger.Init("smtp_things", cfg.jaegerURL, logger)
	defer thingsCloser.Close()

	thConn := clientsgrpc.Connect(cfg.thingsConfig, logger)
	defer thConn.Close()

	tc := thingsapi.NewClient(thConn, thingsTracer, cfg.thingsGRPCTimeout)

	dbTracer, dbCloser := jaeger.Init("smtp_db", cfg.jaegerURL, logger)
	defer dbCloser.Close()

	svc := newService(cfg, logger, dbTracer, db, tc)

	if err = consumers.Start(svcName, pubSub, svc, brokers.SubjectSmtp); err != nil {
		logger.Error(fmt.Sprintf("Failed to create SMTP notifier: %s", err))
	}

	g.Go(func() error {
		return servershttp.Start(ctx, httpapi.MakeHandler(notifiersTracer, svc, logger), cfg.httpConfig, logger)
	})

	g.Go(func() error {
		if sig := errors.SignalHandler(ctx); sig != nil {
			cancel()
			logger.Info(fmt.Sprintf("SMTP notifier service shutdown by signal: %s", sig))
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		logger.Error(fmt.Sprintf("SMTP notifier service terminated: %s", err))
	}

}

func loadConfig() config {
	thingsGRPCTimeout, err := time.ParseDuration(mainflux.Env(envThingsGRPCTimeout, defThingsGRPCTimeout))
	if err != nil {
		log.Fatalf("Invalid %s value: %s", envThingsGRPCTimeout, err.Error())
	}

	thingsTLS, err := strconv.ParseBool(mainflux.Env(envThingsTLS, defThingsTLS))
	if err != nil {
		log.Fatalf("Invalid value passed for %s\n", envThingsTLS)
	}

	emailConf := email.Config{
		FromAddress: mainflux.Env(envEmailFromAddress, defEmailFromAddress),
		FromName:    mainflux.Env(envEmailFromName, defEmailFromName),
		Host:        mainflux.Env(envEmailHost, defEmailHost),
		Port:        mainflux.Env(envEmailPort, defEmailPort),
		Username:    mainflux.Env(envEmailUsername, defEmailUsername),
		Password:    mainflux.Env(envEmailPassword, defEmailPassword),
		Template:    mainflux.Env(envEmailTemplate, defEmailTemplate),
	}

	dbConfig := postgres.Config{
		Host:        mainflux.Env(envDBHost, defDBHost),
		Port:        mainflux.Env(envDBPort, defDBPort),
		User:        mainflux.Env(envDBUser, defDBUser),
		Pass:        mainflux.Env(envDBPass, defDBPass),
		Name:        mainflux.Env(envDB, defDB),
		SSLMode:     mainflux.Env(envDBSSLMode, defDBSSLMode),
		SSLCert:     mainflux.Env(envDBSSLCert, defDBSSLCert),
		SSLKey:      mainflux.Env(envDBSSLKey, defDBSSLKey),
		SSLRootCert: mainflux.Env(envDBSSLRootCert, defDBSSLRootCert),
	}

	httpConfig := servers.Config{
		ServerName:   svcName,
		ServerCert:   mainflux.Env(envServerCert, defServerCert),
		ServerKey:    mainflux.Env(envServerKey, defServerKey),
		Port:         mainflux.Env(envHTTPPort, defHTTPPort),
		StopWaitTime: stopWaitTime,
	}

	thingsConfig := clients.Config{
		ClientTLS:  thingsTLS,
		CaCerts:    mainflux.Env(envThingsCACerts, defThingsCACerts),
		URL:        mainflux.Env(envThingsGRPCURL, defThingsGRPCURL),
		ClientName: clients.Things,
	}

	return config{
		logLevel:          mainflux.Env(envLogLevel, defLogLevel),
		brokerURL:         mainflux.Env(envBrokerURL, defBrokerURL),
		emailConf:         emailConf,
		dbConfig:          dbConfig,
		httpConfig:        httpConfig,
		thingsConfig:      thingsConfig,
		from:              mainflux.Env(envFrom, defFrom),
		jaegerURL:         mainflux.Env(envJaegerURL, defJaegerURL),
		thingsGRPCTimeout: thingsGRPCTimeout,
	}

}

func connectToDB(dbConfig postgres.Config, logger logger.Logger) *sqlx.DB {
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to connect to postgres: %s", err))
		os.Exit(1)
	}
	return db
}

func newService(c config, logger logger.Logger, dbTracer opentracing.Tracer, db *sqlx.DB, tc protomfx.ThingsServiceClient) notifiers.Service {
	idp := uuid.New()
	database := dbutil.NewDatabase(db)

	agent, err := email.New(&c.emailConf)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to create email agent: %s", err))
		os.Exit(1)
	}

	notifier := smtp.New(agent, c.from)
	notifierRepo := postgres.NewNotifierRepository(database)
	notifierRepo = tracing.NotifierRepositoryMiddleware(dbTracer, notifierRepo)
	svc := notifiers.New(idp, notifier, notifierRepo, tc)
	svc = api.LoggingMiddleware(svc, logger)
	svc = api.MetricsMiddleware(
		svc,
		kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "notifier",
			Subsystem: "smtp",
			Name:      "request_count",
			Help:      "Number of requests received.",
		}, []string{"method"}),
		kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
			Namespace: "notifier",
			Subsystem: "smtp",
			Name:      "request_latency_microseconds",
			Help:      "Total duration of requests in microseconds.",
		}, []string{"method"}),
	)

	return svc
}
