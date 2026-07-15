package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"dispatch-system/pb" // 確保這裡的 import 路徑跟你的 go.mod 模組名稱一致

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MachineServer 實作 pb.MachineServiceServer 介面
type MachineServer struct {
	pb.UnimplementedMachineServiceServer

	// 🔒 使用 讀寫鎖 與 map，實作記憶體內的高併發安全資料庫
	mu       sync.RWMutex
	machines map[string]*pb.MachineResponse
}

func NewMachineServer() *MachineServer {
	// 預先塞入幾台模擬的晶圓廠機台（黃光機與蝕刻機）
	initialMachines := map[string]*pb.MachineResponse{
		"EQP-FAB12-LIT01": {
			MachineId:   "EQP-FAB12-LIT01",
			Status:      "IDLE",
			MachineType: "LITHOGRAPHY", // 黃光機
			LastUpdated: time.Now().Unix(),
		},
		"EQP-FAB12-ETC01": {
			MachineId:   "EQP-FAB12-ETC01",
			Status:      "RUNNING",
			MachineType: "ETCHING", // 蝕刻機
			LastUpdated: time.Now().Unix(),
		},
	}

	return &MachineServer{
		machines: initialMachines,
	}
}

// GetMachineStatus 查詢機台狀態 (R 鎖 - 允許多個執行緒同時讀取)
func (s *MachineServer) GetMachineStatus(ctx context.Context, req *pb.MachineRequest) (*pb.MachineResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	machine, exists := s.machines[req.GetMachineId()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "找不到此機台識別碼: %s", req.GetMachineId())
	}

	log.Printf("🔍 [查詢機台] ID: %s, 狀態: %s", machine.MachineId, machine.Status)
	return machine, nil
}

// UpdateMachineStatus 更新機台狀態 (W 鎖 - 寫入時完全互斥，確保資料一致性)
func (s *MachineServer) UpdateMachineStatus(ctx context.Context, req *pb.UpdateStatusRequest) (*pb.MachineResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	machine, exists := s.machines[req.GetMachineId()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "無法更新狀態，找不到機台 ID: %s", req.GetMachineId())
	}

	// 更新狀態與時間戳
	machine.Status = req.GetStatus()
	machine.LastUpdated = time.Now().Unix()

	log.Printf("⚡ [狀態更新] 機台: %s 已變更為: %s", machine.MachineId, machine.Status)
	return machine, nil
}

func main() {
	// 設定 gRPC 監聽的 Port
	port := ":50051"
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("無法監聽連接埠 %s: %v", port, err)
	}

	// 建立 gRPC 伺服器
	grpcServer := grpc.NewServer()
	machineServer := NewMachineServer()

	// 將我們的實作註冊到 gRPC 服務中
	pb.RegisterMachineServiceServer(grpcServer, machineServer)

	fmt.Printf("🚀 晶圓廠機台狀態微服務啟動中，監聽連接埠 %s...\n", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC 伺服器運作失敗: %v", err)
	}
}
