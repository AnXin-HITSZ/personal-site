package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"gorm.io/gorm"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
)

// 口令不从命令行读：参数会出现在 ps 的输出里，同机器的任何用户都看得到，
// 也会留在 shell 历史里。所以反过来——口令由这里生成，只打印一次。
var createAdminEmail = flag.String("create-admin", "",
	"把该邮箱的账号设为管理员，重置口令后退出，不启动服务")

func runCreateAdmin(ctx context.Context, db *gorm.DB, rawEmail string) error {
	email, err := auth.NormalizeEmail(rawEmail)
	if err != nil {
		return fmt.Errorf("-create-admin %s：%w", rawEmail, err)
	}

	password, err := auth.NewPassword()
	if err != nil {
		return errors.New("生成初始口令失败")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return errors.New("生成口令哈希失败")
	}

	repo := repository.NewAccount(db)
	now := time.Now().UTC()

	existing, err := repo.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		id, err := auth.NewID()
		if err != nil {
			return errors.New("生成账号 ID 失败")
		}
		verified := now
		if err := repo.CreateUser(ctx, &model.User{
			ID:              id,
			Email:           email,
			PasswordHash:    hash,
			Role:            model.RoleAdmin,
			Status:          model.UserStatusActive,
			EmailVerifiedAt: &verified,
			CreatedAt:       now,
			UpdatedAt:       now,
		}); err != nil {
			return err
		}
		fmt.Printf("已创建管理员：%s\n", email)
	case err != nil:
		return err
	default:
		// 已经有这个账号就提升并换口令。这一条同时也是唯一的找回手段：
		// 邮件通道没通之前，忘了口令没有别的路可走。
		if err := repo.MakeAdmin(ctx, existing.ID, hash, now); err != nil {
			return err
		}
		fmt.Printf("账号已存在，已提升为管理员并重置口令：%s\n", email)
	}

	fmt.Printf("\n初始口令：%s\n", password)
	fmt.Println("这条口令只显示这一次，请立刻登录并改掉它。")
	return nil
}
