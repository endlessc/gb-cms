package main

import (
	"encoding/json"
	"go.uber.org/zap/zapcore"
	"net"
	"strconv"
)

var (
	Config *Config_
	SipUA  SipServer
	DB     GB28181DB
)

func init() {
	logConfig := LogConfig{
		Level:     int(zapcore.DebugLevel),
		Name:      "./logs/cms.log",
		MaxSize:   10,
		MaxBackup: 100,
		MaxAge:    7,
		Compress:  false,
	}

	InitLogger(zapcore.Level(logConfig.Level), logConfig.Name, logConfig.MaxSize, logConfig.MaxBackup, logConfig.MaxAge, logConfig.Compress)
}

func main() {
	config, err := ParseConfig("./config.json")
	if err != nil {
		panic(err)
	}

	Config = config
	indent, _ := json.MarshalIndent(Config, "", "\t")
	Sugar.Infof("server config:\r\n%s", indent)

	DB = NewRedisDB(Config.Redis.Addr, Config.Redis.Password)

	// 从数据库中恢复会话
	var streams []*Stream
	var sinks []*Sink
	if DB != nil {
		// 查询在线设备, 更新设备在线状态
		updateDevicesStatus()

		// 恢复国标推流会话
		streams, sinks = recoverStreams()
	}

	// 启动sip server
	server, err := StartSipServer(config.SipID, config.ListenIP, config.PublicIP, config.SipPort)
	if err != nil {
		panic(err)
	}

	Sugar.Infof("启动sip server成功. addr: %s:%d", config.ListenIP, config.SipPort)
	Config.SipContactAddr = net.JoinHostPort(config.PublicIP, strconv.Itoa(config.SipPort))
	SipUA = server

	// 在sip启动后, 关闭无效的流
	for _, stream := range streams {
		stream.Close(true, false)
	}

	for _, sink := range sinks {
		sink.Close(true, false)
	}

	// 启动级联设备
	startPlatformDevices()

	httpAddr := net.JoinHostPort(config.ListenIP, strconv.Itoa(config.HttpPort))
	Sugar.Infof("启动http server. addr: %s", httpAddr)
	startApiServer(httpAddr)
}
