package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/config"
	"anxin-hitsz.com/backend/internal/database"
	"anxin-hitsz.com/backend/internal/handler"
	"anxin-hitsz.com/backend/internal/mail"
	"anxin-hitsz.com/backend/internal/middleware"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/ratelimit"
	"anxin-hitsz.com/backend/internal/repository"
	"anxin-hitsz.com/backend/internal/service"
	"anxin-hitsz.com/backend/internal/storage"
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.OpenMySQL(ctx, cfg.MySQL)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Print("关闭 MySQL 连接池失败")
		}
	}()

	// 建管理员是一次性命令，做完就退出——不该顺带把服务也起来，
	// 否则运维还得再想办法把它停掉。
	if *createAdminEmail != "" {
		return runCreateAdmin(ctx, db.DB, *createAdminEmail)
	}

	log.Print("MySQL 连接检查通过")

	// 没配 SMTP 时注册仍然返回 202、仍然看着像成功，只是信永远不会到。
	// 这行日志是唯一能让人提前发现这件事的地方，所以留在启动路径上。
	if cfg.AppEnv == "production" && cfg.Mail == nil {
		log.Print("警告：未配置 SMTP_HOST，注册与找回口令的邮件不会发出")
	}

	router, err := newRouter(cfg, db.DB)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("listening on http://%s", cfg.HTTPAddr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Println("shutdown signal received, draining…")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		if closeErr := srv.Close(); closeErr != nil {
			log.Printf("force close failed: %v", closeErr)
		}
		return err
	}

	log.Println("server stopped")
	return nil
}

// 先看配置，再退到按环境挑的占位实现。配了 SMTP_HOST 就用真的，开发机上
// 也可以配，不必等到生产才第一次试。
//
// 生产没配就用 UnconfiguredMailer：信写不出去，注册和找回口令会明确失败，
// 只留一行日志。失败好过安静地把令牌写进生产日志。
// 没配又落在开发环境才用 LogMailer——它把验证链接打进日志，本地才捞得到令牌。
func newMailer(cfg config.Config) mail.Mailer {
	if cfg.Mail != nil {
		return mail.NewSMTPMailer(*cfg.Mail)
	}
	if cfg.AppEnv == "production" {
		return mail.UnconfiguredMailer{}
	}
	return mail.LogMailer{}
}

// 和 newMailer 同一个形状：配了就用真的，开发环境用只打日志的，生产没配就用
// 一律失败的。上传只有作者一个人用，没配 OSS 时让它在界面上明确报错，
// 好过安静地成功、再给出一串打不开的图片链接。
func newObjectStore(cfg config.Config) storage.Store {
	if cfg.OSS != nil {
		return storage.NewOSSStore(*cfg.OSS)
	}
	if cfg.AppEnv == "production" {
		return storage.UnconfiguredStore{}
	}
	return storage.LogStore{}
}

// 限速策略集中在这里，一眼能看完每条是几分钟几次。
// 具体数字的取舍：按 IP 的额度比按邮箱的宽，因为一个办公网出口后面可能坐着好几个人；
// 按邮箱的额度收紧，因为那才是爆破的靶子。
//
// 第一个参数是这个限速器的名字，会被拼进 key。六个里有三个打在同一个 IP 上、
// 三个打在同一个邮箱上，换个共用的限速存储时要靠这个名字区分开。
func newAccountLimits() handler.AccountLimits {
	return handler.AccountLimits{
		RegisterPerIP:    ratelimit.New("register:ip", 5, time.Hour),
		LoginPerIP:       ratelimit.New("login:ip", 20, 15*time.Minute),
		LoginPerEmail:    ratelimit.New("login:mail", 5, 15*time.Minute),
		PasswordPerIP:    ratelimit.New("password:ip", 10, time.Hour),
		PasswordPerEmail: ratelimit.New("password:mail", 3, time.Hour),
		ResendPerEmail:   ratelimit.New("resend:mail", 3, time.Hour),
	}
}

func newRouter(cfg config.Config, db *gorm.DB) (*gin.Engine, error) {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	// 顺序不能倒：LogServerErrors 在外面，它的 defer 才看得到 Recovery 写下的 500。
	router.Use(middleware.LogServerErrors(), middleware.Recovery())

	if err := router.SetTrustedProxies([]string{"127.0.0.1"}); err != nil {
		return nil, err
	}

	articleService := service.NewArticle(repository.NewArticle(db))
	articleList := handler.NewArticlesList(articleService)
	articleGet := handler.NewArticleGet(articleService)
	adminArticles := handler.NewAdminArticles(articleService)
	uploads := handler.NewUploads(newObjectStore(cfg))

	categoryService := service.NewCategory(repository.NewCategory(db))
	categoriesList := handler.NewCategoriesList(categoryService)
	adminCategories := handler.NewAdminCategories(categoryService)

	accountRepo := repository.NewAccount(db)
	sessionService := service.NewSession(accountRepo)
	accountService := service.NewAccount(accountRepo, sessionService, newMailer(cfg), cfg.SiteBaseURL)

	cookies := middleware.NewSessionCookie(cfg.Session.CookieSecure)
	account := handler.NewAccount(accountService, cookies, newAccountLimits())

	api := router.Group("/api/v1")
	api.Use(middleware.NewOriginPolicy(cfg.Session.AllowedOrigins).SameOrigin())

	api.GET("/articles", articleList.List)
	api.GET("/articles/:id", articleGet.Get)
	// 读者那一行筛选项跟着写作变，所以要问一次；没有登录态。
	api.GET("/categories", categoriesList.List)

	// 没有登录态的接口。它们的响应体都被刻意做成不区分邮箱是否存在。
	auth := api.Group("/auth")
	auth.POST("/register", account.Register)
	auth.POST("/verify-email", account.VerifyEmail)
	auth.POST("/resend-verification", account.ResendVerification)
	auth.POST("/login", account.Login)
	auth.POST("/logout", account.Logout)
	auth.POST("/forgot-password", account.ForgotPassword)
	auth.POST("/reset-password", account.ResetPassword)

	// 要登录的接口。RequireAuth 也在这里被用到真正的业务路由上。
	me := api.Group("/account", middleware.RequireAuth(cookies, sessionService))
	me.GET("/session", account.Session)
	me.POST("/password", account.ChangePassword)
	me.GET("/sessions", account.ListSessions)
	me.DELETE("/sessions/:id", account.RevokeSession)

	// 写作的入口，只有 admin 进得来。注册一律产生 member，没有任何接口能改
	// role——所以这是一道真的门，不是摆样子。
	admin := api.Group("/admin",
		middleware.RequireAuth(cookies, sessionService),
		middleware.RequireRole(model.RoleAdmin))
	admin.GET("/articles", adminArticles.List)
	admin.POST("/articles", adminArticles.Create)
	admin.GET("/articles/:id", adminArticles.Get)
	admin.PUT("/articles/:id", adminArticles.Update)
	admin.DELETE("/articles/:id", adminArticles.Delete)
	admin.POST("/uploads", uploads.Create)

	// order 是 PUT 树上的静态路径，和 PATCH／DELETE 树上的 :id 各在各的树里，撞不上。
	admin.GET("/categories", adminCategories.List)
	admin.POST("/categories", adminCategories.Create)
	admin.PUT("/categories/order", adminCategories.Reorder)
	admin.PATCH("/categories/:id", adminCategories.Rename)
	admin.DELETE("/categories/:id", adminCategories.Delete)

	return router, nil
}
