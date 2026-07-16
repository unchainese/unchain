package main

type Config struct {
	AppHost                 string `def:"svr.libragen.unchain"`                                                      //这个信息会帮助你生成V2ray/Clash/ShadowRocket的订阅链接,同时这个是互联网浏览器访问的地址
	AppPort                 int    `def:"8880"`                                                                      //golang app 服务端口,可选,建议默认80或者443
	RegisterUrl             string `def:"https://unchain.libragen.cn/api/node"`                                      //optional,流量,用户鉴权的主控服务器地址
	RegisterToken           string `def:"unchain people from censorship and surveillance"`                           //optional,流量,用户鉴权的主控服务器token
	AllowUsers              string `def:"903bcd04-79e7-429c-bf0c-0456c7de9cdc,903bcd04-79e7-429c-bf0c-0456c7de9cd1"` //单机模式下,允许的用户UUID
	LogFile                 string `def:""`                                                                          //日志文件路径
	DebugLevel              string `def:"DEBUG"`                                                                     //日志级别
	IntervalSecond          int    `def:"3600"`                                                                      //seconds 向主控服务器推送,流量使用情况的间隔时间
	GitHash                 string `def:""`                                                                          //optional git hash
	BuildTime               string `def:""`                                                                          //optional build time
	RunAt                   string `def:""`                                                                          //optional run at
	EnableDataUsageMetering bool   `def:"true"`                                                                      //是否开启用户流量统计,使用true 开启用户流量统计,使用false 关闭用户流量统计
	BufferSize              int    `def:"8192"`                                                                      //缓冲区大小,用于WebSocket和TCP/UDP读取
}
