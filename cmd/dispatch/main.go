package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"dispatch-system/pb" // 確保這裡與你的 go.mod 模組名稱一致

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DispatchServer struct {
	pb.UnimplementedDispatchServiceServer
	// 用來呼叫機台服務的 gRPC 客戶端
	machineClient pb.MachineServiceClient
}

// DispatchLot 實作派工決策邏輯
func (s *DispatchServer) DispatchLot(ctx context.Context, req *pb.LotRequest) (*pb.DispatchResponse, error) {
	log.Printf("📦 [收到派工請求] 晶圓批次: %s, 所需工序: %s, 交期: %d", req.GetLotId(), req.GetProcessType(), req.GetDueDate())

	// 1. 模擬向 Machine Service 查詢可用機台 (在實務上我們會維護一個機台清單並遍歷它們)
	// 這裡我們直接查詢 EQP-FAB12-LIT01 (黃光機) 與 EQP-FAB12-ETC01 (蝕刻機) 的狀態
	var targetMachine string
	if req.GetProcessType() == "LITHOGRAPHY" {
		targetMachine = "EQP-FAB12-LIT01"
	} else if req.GetProcessType() == "ETCHING" {
		targetMachine = "EQP-FAB12-ETC01"
	} else {
		return &pb.DispatchResponse{
			LotId:             req.GetLotId(),
			AssignedMachineId: "",
			Status:            "FAILED_UNKNOWN_PROCESS",
			DispatchedAt:      time.Now().Unix(),
		}, nil
	}

	// 2. 透過 gRPC 連線到 Machine Service 查詢機台當前狀態
	machineStatus, err := s.machineClient.GetMachineStatus(ctx, &pb.MachineRequest{
		MachineId: targetMachine,
	})
	if err != nil {
		log.Printf("❌ 查詢機台 %s 狀態失敗: %v", targetMachine, err)
		return nil, err
	}

	// 3. 決策邏輯：如果是 IDLE 才可以派工 (這裡就是面試可以吹噓的排程規則處)
	if machineStatus.GetStatus() == "IDLE" {
		log.Printf("🎯 機台 %s 處於空閒狀態，開始進行指派...", targetMachine)

		// 4. 指派成功，透過 gRPC 更新該機台狀態為 RUNNING
		_, err := s.machineClient.UpdateMachineStatus(ctx, &pb.UpdateStatusRequest{
			MachineId: targetMachine,
			Status:    "RUNNING",
		})
		if err != nil {
			log.Printf("❌ 更新機台 %s 狀態至 RUNNING 失敗: %v", targetMachine, err)
			return nil, err
		}

		log.Printf("✅ [派工成功] 晶圓批次 %s 已指派至機台 %s", req.GetLotId(), targetMachine)
		return &pb.DispatchResponse{
			LotId:             req.GetLotId(),
			AssignedMachineId: targetMachine,
			Status:            "SUCCESS",
			DispatchedAt:      time.Now().Unix(),
		}, nil
	}

	// 5. 如果機台忙碌中，派工失敗
	log.Printf("⚠️ 派工失敗：目標機台 %s 目前處於 %s 狀態，無法承接新工作", targetMachine, machineStatus.GetStatus())
	return &pb.DispatchResponse{
		LotId:             req.GetLotId(),
		AssignedMachineId: "",
		Status:            "FAILED_MACHINE_BUSY",
		DispatchedAt:      time.Now().Unix(),
	}, nil
}

func main() {
	// 1. 先建立與 Machine Service (Port 50051) 的 gRPC 連線
	machineServiceAddr := "localhost:50051"
	conn, err := grpc.Dial(machineServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("無法連線至機台微服務: %v", err)
	}
	defer conn.Close()

	// 建立 Machine Service 的 Client
	mClient := pb.NewMachineServiceClient(conn)

	// 2. 啟動 Dispatch Service 本身的 gRPC 伺服器，監聽 Port 50052
	port := ":50052"
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("無法監聽連接埠 %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	dispatchServer := &DispatchServer{machineClient: mClient}

	// 註冊服務
	pb.RegisterDispatchServiceServer(grpcServer, dispatchServer)

	fmt.Printf("🚀 晶圓廠派工決策微服務啟動中，監聽連接埠 %s...\n", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC 伺服器運作失敗: %v", err)
	}
}
