package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"dispatch-system/pb" // 確保這裡與你的 go.mod 模組名稱一致

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1. 連線到 派工微服務 (Port 50052)
	dispatchServiceAddr := "localhost:50052"
	conn, err := grpc.Dial(dispatchServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("無法連線至派工微服務: %v", err)
	}
	defer conn.Close()

	client := pb.NewDispatchServiceClient(conn)

	// 2. 模擬產線源源不絕產生的晶圓批次（Wafer Lots）
	lotsToProcess := []*pb.LotRequest{
		{
			LotId:       "LOT-TSMC-20260715-A1",
			ProcessType: "LITHOGRAPHY", // 需要黃光機
			DueDate:     time.Now().Add(1 * time.Hour).Unix(),
		},
		{
			LotId:       "LOT-TSMC-20260715-A2", // 故意在 A1 還在做的時候，也要求黃光機
			ProcessType: "LITHOGRAPHY",
			DueDate:     time.Now().Add(2 * time.Hour).Unix(),
		},
		{
			LotId:       "LOT-TSMC-20260715-B1",
			ProcessType: "ETCHING", // 需要蝕刻機
			DueDate:     time.Now().Add(30 * time.Minute).Unix(),
		},
	}

	fmt.Println("🎬 晶圓產線模擬腳本啟動，準備發送派工請求...\n")

	for _, lot := range lotsToProcess {
		fmt.Printf("📦 發送晶圓派工請求 -> ID: %s (需求: %s)\n", lot.LotId, lot.ProcessType)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := client.DispatchLot(ctx, lot)
		cancel()

		if err != nil {
			log.Printf("❌ 呼叫派工服務失敗: %v\n\n", err)
			continue
		}

		// 3. 印出派工決策結果
		if resp.Status == "SUCCESS" {
			fmt.Printf("✅ [結果] 派工成功！%s 被指派至機台 %s\n", resp.LotId, resp.AssignedMachineId)
		} else {
			fmt.Printf("⚠️ [結果] 派工拒絕！原因: %s\n", resp.Status)
		}
		fmt.Println("--------------------------------------------------")

		// 稍微間隔一秒，模擬真實產線的流速
		time.Sleep(1 * time.Second)
	}
}
