# Getting Started - 快速开始指南

本文档将指导您从零开始构建、配置和运行微隔离系统。

## 目录

- [系统要求](#系统要求)
- [安装依赖](#安装依赖)
- [构建项目](#构建项目)
- [配置系统](#配置系统)
- [启动服务](#启动服务)
- [验证运行](#验证运行)
- [使用Web界面](#使用web界面)
- [常见问题](#常见问题)

## 系统要求

### 硬件要求
- CPU: 2核或以上
- 内存: 4GB或以上
- 磁盘: 10GB可用空间

### 软件要求

#### 必需组件
- **操作系统**: Linux (Ubuntu 20.04+, CentOS 7+, 或其他现代Linux发行版)
- **Go**: 1.24.0 或以上
- **GCC**: 支持C11标准
- **Node.js**: 18.x 或以上
- **npm**: 8.x 或以上

#### 依赖库
- jansson (JSON库)
- liburcu (RCU库)
- liburcu-cds (RCU数据结构库)
- libnetfilter_queue (Netfilter队列库)
- libpcre2 (正则表达式库)
- pthread
- rt
- m

## 安装依赖

### Ubuntu/Debian

```bash
# 更新包索引
sudo apt-get update

# 安装编译工具
sudo apt-get install -y build-essential gcc make

# 安装Go (如果未安装)
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 安装Node.js (如果未安装)
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs

# 安装C依赖库
sudo apt-get install -y \
    libjansson-dev \
    liburcu-dev \
    libnetfilter-queue-dev \
    libpcre2-dev \
    libpthread-stubs0-dev
```

### CentOS/RHEL

```bash
# 安装编译工具
sudo yum groupinstall -y "Development Tools"

# 安装Go (如果未安装)
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 安装Node.js
curl -fsSL https://rpm.nodesource.com/setup_18.x | sudo bash -
sudo yum install -y nodejs

# 安装C依赖库
sudo yum install -y \
    jansson-devel \
    userspace-rcu-devel \
    libnetfilter_queue-devel \
    pcre2-devel
```

### 验证安装

```bash
# 验证Go
go version
# 应输出: go version go1.24.0 linux/amd64

# 验证GCC
gcc --version
# 应输出: gcc (GCC) 9.x.x 或更高

# 验证Node.js
node --version
# 应输出: v18.x.x

# 验证npm
npm --version
# 应输出: 8.x.x 或更高
```

## 构建项目

### 1. 克隆项目

```bash
git clone https://github.com/your-org/micro-segment.git
cd micro-segment
```

### 2. 下载Go依赖

```bash
go mod download
```

### 3. 构建所有组件

使用顶层Makefile一次性构建所有组件：

```bash
make all
```

这将构建：
- DP (数据平面) - C语言组件
- Agent - Go语言组件
- Controller - Go语言组件

### 4. 单独构建各组件

如果需要单独构建某个组件：

```bash
# 只构建DP
make dp

# 只构建Agent
make agent

# 只构建Controller
make controller

# 只构建Web前端
make web
```

### 5. 验证构建结果

```bash
ls -lh bin/
```

应该看到：
```
bin/
├── dp          (约1.3MB)  - 数据平面可执行文件
├── agent       (约15MB)   - Agent可执行文件
└── controller  (约17MB)   - Controller可执行文件
```

```bash
ls -lh web/dist/
```

应该看到：
```
web/dist/
├── index.html
└── assets/
    ├── index-*.js   (约1.2MB)
    └── index-*.css  (约1KB)
```

## 配置系统

### 1. 创建配置目录

```bash
sudo mkdir -p /etc/micro-segment
sudo mkdir -p /var/log/micro-segment
sudo mkdir -p /var/run/micro-segment
```

### 2. Controller配置

创建 `/etc/micro-segment/controller.yaml`:

```yaml
# Controller配置
http_port: 10443      # REST API端口
grpc_port: 18400      # gRPC端口
log_level: info       # 日志级别: debug, info, warn, error
log_file: /var/log/micro-segment/controller.log
```

### 3. Agent配置

创建 `/etc/micro-segment/agent.yaml`:

```yaml
# Agent配置
controller_addr: localhost:18400  # Controller gRPC地址
dp_socket: /var/run/dp.sock       # DP Unix socket路径
log_level: info
log_file: /var/log/micro-segment/agent.log
```

### 4. 设置权限

```bash
sudo chmod 755 /etc/micro-segment
sudo chmod 644 /etc/micro-segment/*.yaml
sudo chmod 755 /var/log/micro-segment
sudo chmod 755 /var/run/micro-segment
```

## 启动服务

推荐使用systemd管理服务，但也可以直接运行。

### 方式一：直接运行（开发/测试）

#### 1. 启动Controller

```bash
# 前台运行
./bin/controller \
  --http-port 10443 \
  --grpc-port 18400 \
  --log-level info

# 或后台运行
nohup ./bin/controller \
  --http-port 10443 \
  --grpc-port 18400 \
  --log-level info \
  > /var/log/micro-segment/controller.log 2>&1 &
```

#### 2. 启动Agent

```bash
# 前台运行
./bin/agent \
  --grpc-addr localhost:18400 \
  --dp-socket /var/run/dp.sock \
  --log-level info

# 或后台运行
nohup ./bin/agent \
  --grpc-addr localhost:18400 \
  --dp-socket /var/run/dp.sock \
  --log-level info \
  > /var/log/micro-segment/agent.log 2>&1 &
```

#### 3. 启动DP（需要root权限）

```bash
# DP需要操作netfilter，必须以root运行
sudo ./bin/dp
```

#### 4. 启动Web开发服务器（可选）

```bash
cd web
npm run dev
# 访问 http://localhost:5173
```

### 方式二：使用systemd（生产环境推荐）

#### 1. 创建Controller服务

创建 `/etc/systemd/system/micro-segment-controller.service`:

```ini
[Unit]
Description=Micro-Segment Controller
After=network.target

[Service]
Type=simple
User=root
ExecStart=/opt/micro-segment/bin/controller \
  --http-port 10443 \
  --grpc-port 18400 \
  --log-level info
Restart=on-failure
RestartSec=5s
StandardOutput=append:/var/log/micro-segment/controller.log
StandardError=append:/var/log/micro-segment/controller.log

[Install]
WantedBy=multi-user.target
```

#### 2. 创建Agent服务

创建 `/etc/systemd/system/micro-segment-agent.service`:

```ini
[Unit]
Description=Micro-Segment Agent
After=network.target micro-segment-controller.service
Requires=micro-segment-controller.service

[Service]
Type=simple
User=root
ExecStart=/opt/micro-segment/bin/agent \
  --grpc-addr localhost:18400 \
  --dp-socket /var/run/dp.sock \
  --log-level info
Restart=on-failure
RestartSec=5s
StandardOutput=append:/var/log/micro-segment/agent.log
StandardError=append:/var/log/micro-segment/agent.log

[Install]
WantedBy=multi-user.target
```

#### 3. 创建DP服务

创建 `/etc/systemd/system/micro-segment-dp.service`:

```ini
[Unit]
Description=Micro-Segment Data Plane
After=network.target
Before=micro-segment-agent.service

[Service]
Type=simple
User=root
ExecStart=/opt/micro-segment/bin/dp
Restart=on-failure
RestartSec=5s
StandardOutput=append:/var/log/micro-segment/dp.log
StandardError=append:/var/log/micro-segment/dp.log

[Install]
WantedBy=multi-user.target
```

#### 4. 启用并启动服务

```bash
# 复制二进制文件到系统目录
sudo mkdir -p /opt/micro-segment/bin
sudo cp bin/* /opt/micro-segment/bin/
sudo chmod +x /opt/micro-segment/bin/*

# 重新加载systemd
sudo systemctl daemon-reload

# 启用服务（开机自启）
sudo systemctl enable micro-segment-controller
sudo systemctl enable micro-segment-agent
sudo systemctl enable micro-segment-dp

# 启动服务
sudo systemctl start micro-segment-dp
sudo systemctl start micro-segment-controller
sudo systemctl start micro-segment-agent

# 查看服务状态
sudo systemctl status micro-segment-controller
sudo systemctl status micro-segment-agent
sudo systemctl status micro-segment-dp
```

## 验证运行

### 1. 检查进程

```bash
ps aux | grep -E "(controller|agent|dp)" | grep -v grep
```

应该看到三个进程在运行。

### 2. 检查端口

```bash
# 检查Controller REST API端口
sudo netstat -tlnp | grep 10443

# 检查Controller gRPC端口
sudo netstat -tlnp | grep 18400
```

### 3. 健康检查

```bash
# Controller健康检查
curl http://localhost:10443/health

# 应返回: {"status":"ok"}
```

### 4. 获取系统统计

```bash
curl http://localhost:10443/api/v1/stats
```

应返回类似：
```json
{
  "workloads": 0,
  "groups": 1,
  "policies": 0,
  "connections": 0
}
```

### 5. 查看日志

```bash
# Controller日志
tail -f /var/log/micro-segment/controller.log

# Agent日志
tail -f /var/log/micro-segment/agent.log

# DP日志（如果有）
tail -f /var/log/micro-segment/dp.log

# 使用systemd时查看日志
sudo journalctl -u micro-segment-controller -f
sudo journalctl -u micro-segment-agent -f
sudo journalctl -u micro-segment-dp -f
```

## 使用Web界面

### 1. 生产环境部署

使用Nginx部署Web前端：

#### 安装Nginx

```bash
sudo apt-get install nginx  # Ubuntu/Debian
# 或
sudo yum install nginx       # CentOS/RHEL
```

#### 配置Nginx

创建 `/etc/nginx/sites-available/micro-segment`:

```nginx
server {
    listen 80;
    server_name your-domain.com;  # 替换为你的域名或IP

    # Web前端
    location / {
        root /opt/micro-segment/web;
        index index.html;
        try_files $uri $uri/ /index.html;
    }

    # 代理API请求到Controller
    location /api/ {
        proxy_pass http://localhost:10443;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /health {
        proxy_pass http://localhost:10443;
    }
}
```

#### 部署Web文件

```bash
# 复制构建产物到Nginx目录
sudo mkdir -p /opt/micro-segment/web
sudo cp -r web/dist/* /opt/micro-segment/web/

# 启用站点
sudo ln -s /etc/nginx/sites-available/micro-segment /etc/nginx/sites-enabled/

# 测试配置
sudo nginx -t

# 重启Nginx
sudo systemctl restart nginx
```

#### 访问Web界面

打开浏览器访问: `http://your-domain.com` 或 `http://your-server-ip`

### 2. 开发环境

```bash
cd web
npm run dev
```

访问: `http://localhost:5173`

### 3. Web界面功能

登录后可以访问：

- **仪表盘** (`/`) - 系统概览
- **网络拓扑** (`/graph`) - 可视化网络连接
- **工作负载** (`/workloads`) - 容器列表
- **组管理** (`/groups`) - 管理容器组
- **策略管理** (`/policies`) - 网络策略配置
- **连接监控** (`/connections`) - 实时连接监控

## 常见问题

### Q1: 编译时找不到某个库

**问题**: `fatal error: jansson.h: No such file or directory`

**解决**:
```bash
# Ubuntu/Debian
sudo apt-get install libjansson-dev

# CentOS/RHEL
sudo yum install jansson-devel
```

### Q2: DP无法启动

**问题**: `Permission denied` 或 `Cannot open netfilter queue`

**解决**: DP需要root权限访问netfilter
```bash
sudo ./bin/dp
```

### Q3: Agent连接不到Controller

**问题**: `Failed to connect to controller`

**解决**:
1. 确认Controller已启动: `curl http://localhost:10443/health`
2. 检查防火墙设置，确保18400端口开放
3. 检查gRPC地址配置是否正确

### Q4: Web界面无法访问API

**问题**: `Network Error` 或 `CORS error`

**解决**:
1. 开发模式：确保在 `vite.config.ts` 中配置了正确的proxy
2. 生产模式：使用Nginx反向代理，避免CORS问题
3. 检查Controller REST API端口是否正确

### Q5: npm install失败

**问题**: `npm ERR! network timeout`

**解决**:
```bash
# 使用国内镜像
npm config set registry https://registry.npmmirror.com
npm install
```

### Q6: 构建时内存不足

**问题**: `virtual memory exhausted: Cannot allocate memory`

**解决**:
```bash
# 增加交换空间
sudo dd if=/dev/zero of=/swapfile bs=1M count=2048
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

### Q7: systemd服务无法启动

**问题**: Service failed to start

**解决**:
```bash
# 查看详细日志
sudo journalctl -xeu micro-segment-controller.service

# 检查二进制文件路径和权限
ls -l /opt/micro-segment/bin/

# 手动运行测试
sudo /opt/micro-segment/bin/controller --log-level debug
```

### Q8: Web构建后文件过大

**问题**: `chunk size limit warning`

**解决**: 这是性能建议，不影响功能。如需优化：
1. 在 `vite.config.ts` 中配置代码分割
2. 使用动态import()
3. 调整 `build.chunkSizeWarningLimit`

## 下一步

现在你已经成功运行了微隔离系统！接下来可以：

1. **配置网络策略** - 参考 [docs/policy-guide.md](docs/policy-guide.md)
2. **集成容器平台** - 参考 [docs/integration.md](docs/integration.md)
3. **性能调优** - 参考 [docs/performance.md](docs/performance.md)
4. **监控和告警** - 参考 [docs/monitoring.md](docs/monitoring.md)

## 支持

- 提交Issue: https://github.com/your-org/micro-segment/issues
- 文档: https://github.com/your-org/micro-segment/tree/main/docs
- 社区讨论: https://github.com/your-org/micro-segment/discussions

## 许可证

Apache License 2.0
