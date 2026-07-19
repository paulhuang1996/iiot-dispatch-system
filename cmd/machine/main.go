package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"dispatch-system/pb" // 確保與 go.mod 一致

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MachineServer struct {
	pb.UnimplementedMachineServiceServer
	// 🔌 改為注入 Redis Client
	rdb *redis.Client
}

func NewMachineServer(rdb *redis.Client) *MachineServer {
	ctx := context.Background()

	// 預先塞入模擬晶圓廠機台到 Redis 中進行初始化
	machines := []struct {
		id    string
		mType string
	}{
		{"EQP-FAB12-LIT01", "LITHOGRAPHY"},
		{"EQP-FAB12-ETC01", "IDLE"}, // 上一步 debug 改為 IDLE
	}

	for _, m := range machines {
		// 檢查 Redis 是否已有資料，沒有才初始化
		exists, _ := rdb.Exists(ctx, m.id).Result()
		if exists == 0 {
			data := &pb.MachineResponse{
				MachineId:   m.id,
				Status:      "IDLE",
				MachineType: m.mType,
				LastUpdated: time.Now().Unix(),
			}
			jsonData, _ := json.Marshal(data)
			rdb.Set(ctx, m.id, jsonData, 0) // 0 代表永久保存
		}
	}

	return &MachineServer{rdb: rdb}
}

// GetMachineStatus 從 Redis 讀取狀態
func (s *MachineServer) GetMachineStatus(ctx context.Context, req *pb.MachineRequest) (*pb.MachineResponse, error) {
	val, err := s.rdb.Get(ctx, req.GetMachineId()).Result()
	if err == redis.Nil {
		return nil, status.Errorf(codes.NotFound, "Redis 中找不到此機台識別碼: %s", req.GetMachineId())
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "Redis 連線異常: %v", err)
	}

	// 將 Redis 撈出來的 JSON 字串反序列化回 Protobuf 結構體
	var machine pb.MachineResponse
	if err := json.Unmarshal([]byte(val), &machine); err != nil {
		return nil, status.Errorf(codes.Internal, "資料解析失敗: %v", err)
	}

	log.Printf("🔍 [Redis 查詢] ID: %s, 狀態: %s", machine.MachineId, machine.Status)
	return &machine, nil
}

// UpdateMachineStatus 在 Redis 中更新狀態
func (s *MachineServer) UpdateMachineStatus(ctx context.Context, req *pb.UpdateStatusRequest) (*pb.MachineResponse, error) {
	// 1. 先獲取原有資料以保留 MachineType
	val, err := s.rdb.Get(ctx, req.GetMachineId()).Result()
	if err == redis.Nil {
		return nil, status.Errorf(codes.NotFound, "無法更新，找不到機台 ID: %s", req.GetMachineId())
	}

	var machine pb.MachineResponse
	json.Unmarshal([]byte(val), &machine)

	// 2. 修改狀態與時間戳
	machine.Status = req.GetStatus()
	machine.LastUpdated = time.Now().Unix()

	// 3. 寫回 Redis
	jsonData, _ := json.Marshal(&machine)
	err = s.rdb.Set(ctx, req.GetMachineId(), jsonData, 0).Err()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Redis 寫入失敗: %v", err)
	}

	log.Printf("⚡ [Redis 更新] 機台: %s 已變更為: %s", machine.MachineId, machine.Status)
	return &machine, nil
}

func main() {
	// 初始化 Redis 連線
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 預設無密碼
		DB:       0,  // 使用預設 DB 0
	})

	// 測試 Redis 連線是否正常
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("❌ 無法連線至 Redis 伺服器: %v", err)
	}

	port := ":50051"
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("無法監聽連接埠 %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	machineServer := NewMachineServer(rdb)

	pb.RegisterMachineServiceServer(grpcServer, machineServer)

	fmt.Printf("🚀 [Redis 版本] 晶圓廠機台狀態微服務啟動，監聽連接埠 %s...\n", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC 伺服器運作失敗: %v", err)
	}
}
