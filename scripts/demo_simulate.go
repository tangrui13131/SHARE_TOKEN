package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func main() {
	_ = common.InitRedisClient()
	if err := model.InitDB(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	if err := model.InitLogDB(); err != nil {
		log.Fatalf("初始化日志库失败: %v", err)
	}

	// 目标演示用户：默认为当前管理员/用户 tr (ID: 1)，也支持命令行传参
	targetUserId := 1
	if len(os.Args) > 1 {
		if id, err := strconv.Atoi(os.Args[1]); err == nil && id > 0 {
			targetUserId = id
		}
	}

	var user model.User
	if err := model.DB.First(&user, targetUserId).Error; err != nil {
		log.Fatalf("未找到用户 ID %d: %v", targetUserId, err)
	}

	// 查找该用户下的一个渠道，若没有则自动创建一个演示渠道
	var ch model.Channel
	err := model.DB.Where("user_id = ?", targetUserId).Order("id desc").First(&ch).Error
	if err != nil {
		fmt.Printf("💡 用户 [%s] 名下暂无渠道，正在自动创建一条演示共享渠道...\n", user.Username)
		demoCh := model.Channel{
			Name:        "GPT-4o 闲置托管共享通道 (演示)",
			Type:        constant.ChannelTypeOpenAI,
			Key:         "sk-proj-demoKey888888888888888888",
			UserId:      &targetUserId,
			Group:       "default",
			Models:      "gpt-4o,gpt-4o-mini",
			Status:      common.ChannelStatusEnabled,
			CreatedTime: common.GetTimestamp(),
		}
		weight := uint(1)
		demoCh.Weight = &weight
		if err := model.BatchInsertChannels([]model.Channel{demoCh}); err != nil {
			log.Fatalf("创建演示渠道失败: %v", err)
		}
		model.DB.Where("user_id = ?", targetUserId).Order("id desc").First(&ch)
		fmt.Printf("✅ 已创建演示渠道: [%s] (ID: %d)\n", ch.Name, ch.Id)
	}

	// 模拟一次调用消费：例如消费 500,000 配额 ($1.00 USD)
	consumeQuota := 500000
	fmt.Println("\n==================================================")
	fmt.Printf("🎯 演示操作: 模拟消费者调用渠道 [%s](ID:%d)\n", ch.Name, ch.Id)
	fmt.Printf("💸 产生 API 消费: %d 配额 ($1.00 美元)\n", consumeQuota)
	fmt.Println("==================================================")

	oldQuota := user.Quota
	model.UpdateChannelUsedQuota(ch.Id, consumeQuota)

	var updatedUser model.User
	model.DB.First(&updatedUser, targetUserId)
	earned := updatedUser.Quota - oldQuota

	fmt.Println("🎉 结算引擎执行完毕！")
	fmt.Printf("  -> 贡献者原余额: %d\n", oldQuota)
	fmt.Printf("  -> 贡献者新余额: %d (+%d 收益，折合 $0.80 美元)\n", updatedUser.Quota, earned)
	fmt.Println("\n👉 请立即在浏览器中刷新页面查看效果！")
}
