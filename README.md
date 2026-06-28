---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: da6502ccb5c2f99d7dcc9554f9e79abc_ff657888730a11f1986d525400d9a7a1
    ReservedCode1: gsdCiUF+fn6SsHFsUDKz63WkboBmYm9boZCe8wcodXMDpQQqVeGowH5Ya0bLNoOrBWzBXSM0ayEsZ6U4k12YHUsk9Us9clAnpRRKJgXcpD2K8snmdicguc3fl2C4YxOkRhaX8GbPnK5lAj5xDIkARljetkpCPX1UuI59cYf+2jJoki1/vZYnCOWqKgg=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: da6502ccb5c2f99d7dcc9554f9e79abc_ff657888730a11f1986d525400d9a7a1
    ReservedCode2: gsdCiUF+fn6SsHFsUDKz63WkboBmYm9boZCe8wcodXMDpQQqVeGowH5Ya0bLNoOrBWzBXSM0ayEsZ6U4k12YHUsk9Us9clAnpRRKJgXcpD2K8snmdicguc3fl2C4YxOkRhaX8GbPnK5lAj5xDIkARljetkpCPX1UuI59cYf+2jJoki1/vZYnCOWqKgg=
---

# config

一个通用的 Go 应用配置管理库。

## 快速开始

### 1. 定义配置结构体

```go
type MyConfig struct {
    Server   ServerConfig        `mapstructure:"server"`
    Database DatabaseConfig      `mapstructure:"database"`
    Apollo   config.RemoteConfig `mapstructure:"apollo"`
}
```

### 2. 初始化 App 并加载本地配置

```go
app := config.New()
cfg := &MyConfig{}
if err := app.LoadLocal("./local", cfg); err != nil {
    log.Fatal(err)
}
```

### 3. （可选）连接远程配置中心

```go
rc := config.ParseRemoteConfig(app.Viper(), "apollo")
if rc != nil {
    provider, _ := apollo.New(rc)
    if err := app.UseRemote(provider); err != nil {
        log.Printf("远程配置: %v", err)
    }
}
```

### 4. 注册变更监听

```go
app.OnChange(func(raw any) {
    newCfg := raw.(*MyConfig)
    log.Printf("配置已变更，新端口: %d", newCfg.Server.Port)
})
```

### 5. 获取配置

```go
currentCfg := app.MustGet().(*MyConfig)
fmt.Println(currentCfg.Server.Host)
```

## 本地配置文件约定

配置文件放置在应用的 `local/` 目录下。命名格式：`app-{env}.{ext}` 或 `app.{ext}`（默认）。

支持的后缀及优先级：`.yml` / `.yaml` > `.json` > `.properties`

示例目录结构：

```
local/
├── app.yml           ← 默认配置（最低优先级，作为兜底）
├── app-dev.yml       ← 开发环境
├── app-prod.yml      ← 生产环境
└── app-test.json     ← 测试环境（json 格式）
```

## 远程配置

在本地配置中声明远程配置中心的连接信息即可自动启用：

```yaml
# config.yaml
apollo:
  type: apollo
  app_id: my-app
  cluster: default
  namespace: application
  ip: http://apollo-config.example.com:8080
  sync_interval: 60

etcd:
  type: etcd
  endpoints:
    - http://etcd-1:2379
    - http://etcd-2:2379
  namespace: my-app
```

## 可用的 Provider

- `providers/apollo`：Apollo 配置中心（HTTP 拉取 + 定时轮询）
- `providers/etcd`：Etcd 配置中心（前缀扫描 + Watch）

如需接入其他配置中心（Consul、Nacos 等），实现 `config.RemoteProvider` 接口即可。
*（内容由AI生成，仅供参考）*
