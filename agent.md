# agent.md — New API (闲置 API Key 共享与分账系统) Agent 开发指南与规范

本文档为 AI Agent 以及开发者提供项目架构概览、核心业务逻辑、开发规范与维护指南。在执行任何代码规划、修改、审查或回答项目问题前，必须以此规范为准。

---

## 一、项目概览与背景

### 1.1 项目定位
本项目基于 **New API**（聚合 40+ 上游大模型服务商的统一 AI 网关与调度分发系统），针对软件工程课程设计/实训需求进行了深度二次开发，核心新增了**去中心化的“闲置 API Key 托管共享与 80% 流量分成系统”**。

- **基础平台**：支持 OpenAI、Claude、Gemini、DeepSeek、Azure 等 40+ 厂商协议聚合、令牌管理、用户体系、高并发中转与计费。
- **核心改造特性**：
  1. **普通用户 Key 共享**：普通用户可上传闲置 API Key，纳入平台公共路由池（`default` 分组）。
  2. **智能加权负载均衡**：网关在调度外部消费请求时，自动基于加权随机算法命中共享渠道。
  3. **80% 自动分账结算引擎**：当消费者通过该 Key 产生调用并消耗额度时，结算引擎实时将消费金额的 **80%** 作为配额返还到贡献者钱包余额。
  4. **安全与看板**：全链路密钥脱敏（`sk-pro******a1b2`）、在线测活、实时收益看板与入账明细账单。

### 1.2 技术栈

| 领域 | 技术选型 | 说明 |
| :--- | :--- | :--- |
| **后端语言** | Go 1.25.1 | 见根目录及子模块 `go.mod` |
| **后端框架** | Gin Web Framework + GORM v2 | 负责网关路由、业务控制器与 ORM 映射 |
| **前端框架** | React 19 + TypeScript + Rsbuild 2 | 现代化单页应用架构（位于 `web/` 目录） |
| **前端状态与路由** | TanStack Router + Query + Table + Zustand | 高性能数据请求缓存与声明式路由 |
| **前端 UI & 样式** | Base UI + Tailwind CSS 4 + Hugeicons | 无障碍基础组件与实用类样式 |
| **前端包管理** | Bun (推荐) | 优先使用 `bun` 运行依赖安装与脚本 |
| **核心数据库** | SQLite / MySQL >= 5.7.8 / PostgreSQL >= 9.6 | 必须保持三大关系型数据库完全兼容 |
| **缓存系统** | Redis (go-redis) + 内存缓存 | 支持单机无 Redis 环境防御性回退 |
| **协议中转** | `relaykit/` | 独立 Go 模块，处理各模型厂商 DTO 转换 |

---

## 二、项目架构与目录导览

```
new-api-main/
├── controller/             # 控制器层：处理 HTTP 请求与业务分发
│   ├── channel.go          # 管理员渠道管理
│   ├── user_channel.go     # [核心改造] 普通用户闲置 Key 共享、测试、启停与收益统计
│   ├── relay.go            # AI 网关核心请求中转入口
│   └── user.go             # 用户信息、配额管理
├── model/                  # 数据模型层与 GORM 数据库交互
│   ├── ability.go          # 渠道模型能力映射表（abilities）
│   ├── channel.go          # [核心改造] 渠道模型，包含 80% 分账结算逻辑 (UpdateChannelUsedQuota)
│   ├── user.go             # 用户模型与配额更新 (IncreaseUserQuota)
│   └── log.go              # 消费与入账明细日志记录
├── relay/                  # 网关核心层：多厂商协议适配器与路由调度
├── relaykit/               # 独立的 Go 模块，存放协议 DTO 与转换工具（严禁反向依赖根模块）
├── router/                 # 路由注册层
│   └── api-router.go       # 注册 /api/user-channel 等核心接口
├── service/                # 业务逻辑服务层（计费、额度校验、通知、安全校验等）
├── web/                    # React 19 前端工程（详见 web/AGENTS.md）
│   ├── src/
│   │   ├── features/       # 业务特性模块
│   │   │   └── user-channels/ # [核心改造] 闲置 Key 托管页面与实时收益看板卡片
│   │   ├── components/     # 全局复用 UI 组件
│   │   └── i18n/           # 国际化配置与多语言字典
│   └── package.json
├── demo.bat                # 一键运行与演示脚本
├── PROJECT_GUIDE.md        # 课程设计项目详细指南与答辩演示手册
└── main.go                 # 后端入口文件
```

---

## 三、核心业务逻辑与规范（闲置 Key 共享与分账）

### 3.1 业务时序与分账流转
1. **上传入池**：
   - 用户通过前端提交 API Key 并指定支持的模型（如 `gpt-4o`）。
   - 后端 `controller/user_channel.go` 接收请求，自动绑定当前登录用户的 `UserId`，强制打上 `default` 分组标签，赋予调度权重 `weight = 1`。
   - 数据写入 `channels` 表并在 `abilities` 表中建立模型路由映射。
2. **网关命中与调度**：
   - 外部调用者发起 API 请求（如调用 `gpt-4o` 消费配额 500,000 点，即 $1.00）。
   - 网关在 `default` 分组下检索具备该能力的活跃渠道，通过加权随机算法调度至该用户渠道。
3. **80% 实时结算分成**：
   - 调用成功后，网关触发 `UpdateChannelUsedQuota(channel_id, quota)`。
   - 结算引擎检测到该渠道 `UserId > 0`（即属于用户贡献渠道），按消费金额的 80% 计算分账：
     $$\text{奖励额度} = \text{消耗额度} \times 0.8$$
   - 实时调用 `IncreaseUserQuota` 为贡献者充值，并在 `logs` 表记录一条清晰的入账流水。
4. **密钥脱敏与安全**：
   - 任何涉及用户渠道列表的 API 查询，密钥必须经由掩码脱敏函数（仅保留前缀与尾部字符，如 `sk-pro******a1b2`）。

---

## 四、Agent 编码规范与准则

### 4.1 通用代码规范
- **简洁直观**：优先采用提前返回（Early Return）、清晰分支和语义明确的局部变量，杜绝深层嵌套。
- **避免多余抽象**：禁止为仅有一处调用的逻辑抽取多余的包级/模块级辅助函数；若抽取函数，其命名必须代表稳定的业务概念。
- **项目受保护信息**：
  - 严禁随意删除、篡改或覆盖项目中关于 **new-api** 以及 **QuantumNous** 的开源作者归属与品牌标识。

### 4.2 后端 Go 规范
1. **现代 Go 语法**：
   - 统一使用 `any` 替代 `interface{}`。
   - 切片检查使用 `slices.Contains` / `slices.ContainsFunc`，字符串处理优先使用 `strings.Cut`、`strings.CutPrefix`、`strings.CutSuffix`。
2. **JSON 处理强制规则**：
   - **严禁**在根模块的业务代码中直接导入或调用标准库 `encoding/json` 的序列化/反序列化函数。
   - **必须**统一使用 `common/json.go` 中的封装：
     - `common.Marshal(v any) ([]byte, error)`
     - `common.Unmarshal(data []byte, v any) error`
     - `common.UnmarshalJsonStr(data string, v any) error`
     - `common.DecodeJson(reader io.Reader, v any) error`
3. **三数据库兼容性原则**：
   - 所有数据库操作必须同时兼容 **SQLite**、**MySQL >= 5.7.8** 和 **PostgreSQL >= 9.6**。
   - 优先使用 GORM 内置方法（`Create`、`Find`、`Where`、`Updates` 等），禁止编写包含方言特性的原生 SQL。
   - 主键生成交由 GORM 处理，严禁直接使用 `AUTO_INCREMENT` 或 `SERIAL`。
4. **relaykit 模块独立性**：
   - `relaykit/` 为独立 Go 模块，**严禁**导入或依赖根模块 `new-api` 的任何包。
5. **认证与安全（OWASP 强制规范）**：
   - 涉及登录、注册、Token 签发、密码重置等认证流程，必须遵循 OWASP ASVS 与 Session Management Cheat Sheet。
   - 权限校验必须在服务端强制执行，敏感凭证在日志与前端展示时必须严格脱敏。

### 4.3 前端 React & TypeScript 规范
1. **包管理与命令**：
   - 前端所有操作在 `web/` 目录下执行，优先且强制使用 **Bun**（`bun install`、`bun run dev`、`bun run build`）。
2. **UI 组件复用原则**：
   - 在新增前端 UI 前，必须先检索 `web/src/components/`，优先复用现有业务组件（如 `CopyButton`、`ConfirmDialog` 等）及 Base UI / Tailwind 基础组件，严禁重复造轮子。
3. **国际化（i18n）**：
   - 界面所有面向用户的文案均需通过 `useTranslation()` 和 `t('English key')` 处理。
   - 语言字典维护在 `web/src/i18n/locales/{lang}.json`（以英文短语作为 key，配置对应的 zh、en 翻译）。
4. **格式化与数值处理**：
   - 普通数值使用 `@/lib/format`，金额/配额显示使用 `@/lib/currency`。
   - 传递给 `Intl.*` 或 `toLocaleString` 的语言代码必须经过 `@/i18n/languages` 中的 `toIntlLocale` 转换。

---

## 五、常用运行与调试命令

### 5.1 本地快速拉起（开发环境）
```powershell
# 根目录下执行一键演示脚本（自动配置端口与环境）
.\demo.bat
```

### 5.2 后端命令
```powershell
# 编译后端可执行文件
go build -ldflags "-s -w" -o new-api.exe

# 运行后端（指定端口与数据库）
.\new-api.exe --port 3000 --log-dir ./logs
```

### 5.3 前端命令
```powershell
# 进入前端目录
cd web

# 安装依赖
bun install

# 启动开发服务器（热更新）
bun run dev

# 执行代码风格与语法检查
bun run lint

# 生产环境打包构建
bun run build
```

---

## 六、任务处理检查清单 (Agent Checklist)

当 Agent 完成需求开发或缺陷修复后，请对照以下清单进行自检：

- [ ] 是否确认了 Go 后端 JSON 操作全部使用了 `common.Marshal` / `common.Unmarshal`？
- [ ] 数据表与 GORM 变更是否同时兼容 SQLite、MySQL 和 PostgreSQL？
- [ ] 用户闲置 Key 相关改动是否确保了 `UserId` 绑定以及 80% 分成结算逻辑的正确性？
- [ ] 前端界面改动是否优先复用了既有组件并完成了 `i18n` 国际化覆盖？
- [ ] 密钥与敏感信息是否在前端展示与日志打印中均做了脱敏处理？
- [ ] 是否在本地执行了编译校验（`go build` / `bun run build`）确保零错误？
