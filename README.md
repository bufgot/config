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

nacos:
  type: nacos
  endpoints:
    - http://127.0.0.1:8848
  namespace: my-namespace
  app_id: my-config
  extra:
    dataId: my-config-data-id
    group: DEFAULT_GROUP
  sync_interval: 30
```

## 可用的 Provider

### Apollo

`providers/apollo` — Apollo 配置中心，通过 HTTP REST API 拉取配置，定时轮询 detectionId 检测变更。

配置字段：

| 字段 | 必填 | 说明 |
|------|------|------|
| `endpoints` | 是 | Apollo Config Service 地址列表 |
| `app_id` | 是 | 应用 ID |
| `cluster` | 否 | 集群名（默认 default） |
| `namespace` | 否 | 命名空间（默认 application） |
| `sync_interval` | 否 | 同步间隔秒数（默认 60） |

### Etcd

`providers/etcd` — Etcd 配置中心，通过前缀扫描 + 原生 Watch 机制实时推送变更。

配置字段：

| 字段 | 必填 | 说明 |
|------|------|------|
| `endpoints` | 是 | Etcd 节点地址列表 |
| `namespace` | 否 | 配置 key 前缀（默认 /config/） |
| `app_id` | 否 | 配置 key 前缀备选（当 namespace 为空时使用） |

### Nacos

`providers/nacos` — Nacos 配置中心，通过 HTTP Open API (`/v1/cs/configs`) 拉取配置，定时轮询 + MD5 哈希检测配置变更。

配置字段：

| 字段 | 必填 | 说明 |
|------|------|------|
| `endpoints` | 是 | Nacos 服务地址，如 `http://127.0.0.1:8848` |
| `app_id` 或 `extra.dataId` | 是 | 配置 Data ID，`extra.dataId` 优先级高于 `app_id` |
| `namespace` | 否 | Nacos 命名空间（租户 ID），空表示 public |
| `extra.group` | 否 | 配置分组（默认 `DEFAULT_GROUP`） |
| `sync_interval` | 否 | 轮询间隔秒数（默认 30） |

代码示例：

```go
import "github.com/bufgot/config/providers/nacos"

rc := config.ParseRemoteConfig(app.Viper(), "nacos")
if rc != nil {
    provider, err := nacos.New(rc)
    if err != nil {
        log.Fatal(err)
    }
    app.UseRemote(provider)
}
```

如需接入其他配置中心（Consul 等），实现 `config.RemoteProvider` 接口即可。
## Test Coverage

Overall module: **80%+**

## Security

| Vulnerability | Status |
|---|---|
| grpc | Fixed |
| quic-go | Fixed |
| crypto/tls | Pending Go 1.26.5 |

*（内容由AI生成，仅供参考）*
