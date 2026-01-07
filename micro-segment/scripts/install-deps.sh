#!/bin/bash

# 微隔离项目依赖安装脚本

set -e

echo "=== 安装微隔离项目依赖 ==="

# 检查操作系统
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$NAME
    VER=$VERSION_ID
else
    echo "无法检测操作系统版本"
    exit 1
fi

echo "检测到操作系统: $OS $VER"

# Ubuntu/Debian系统
if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
    echo "安装Ubuntu/Debian依赖..."
    
    # 更新包列表
    sudo apt-get update
    
    # 安装编译工具
    sudo apt-get install -y \
        build-essential \
        gcc \
        make \
        pkg-config \
        git
    
    # 安装Go (1.20+)
    if ! command -v go &> /dev/null; then
        echo "安装Go..."
        wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
        sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        export PATH=$PATH:/usr/local/go/bin
        rm go1.21.5.linux-amd64.tar.gz
    fi
    
    # 安装C库依赖
    sudo apt-get install -y \
        libjansson-dev \
        liburcu-dev \
        libnetfilter-queue-dev \
        libnetfilter-queue1 \
        libnfnetlink-dev \
        iptables-dev \
        libpcap-dev
    
    # 安装Node.js和npm
    if ! command -v node &> /dev/null; then
        echo "安装Node.js..."
        curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
        sudo apt-get install -y nodejs
    fi
    
    # 安装Docker
    if ! command -v docker &> /dev/null; then
        echo "安装Docker..."
        curl -fsSL https://get.docker.com -o get-docker.sh
        sudo sh get-docker.sh
        sudo usermod -aG docker $USER
        rm get-docker.sh
    fi
    
    # 安装Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        echo "安装Docker Compose..."
        sudo curl -L "https://github.com/docker/compose/releases/download/v2.21.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
        sudo chmod +x /usr/local/bin/docker-compose
    fi

# CentOS/RHEL系统
elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
    echo "安装CentOS/RHEL依赖..."
    
    # 安装EPEL仓库
    sudo yum install -y epel-release
    
    # 安装编译工具
    sudo yum groupinstall -y "Development Tools"
    sudo yum install -y \
        gcc \
        make \
        pkg-config \
        git
    
    # 安装Go
    if ! command -v go &> /dev/null; then
        echo "安装Go..."
        wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
        sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        export PATH=$PATH:/usr/local/go/bin
        rm go1.21.5.linux-amd64.tar.gz
    fi
    
    # 安装C库依赖
    sudo yum install -y \
        jansson-devel \
        userspace-rcu-devel \
        libnetfilter_queue-devel \
        libnfnetlink-devel \
        iptables-devel \
        libpcap-devel
    
    # 安装Node.js
    if ! command -v node &> /dev/null; then
        echo "安装Node.js..."
        curl -fsSL https://rpm.nodesource.com/setup_18.x | sudo bash -
        sudo yum install -y nodejs
    fi
    
    # 安装Docker
    if ! command -v docker &> /dev/null; then
        echo "安装Docker..."
        sudo yum install -y yum-utils
        sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
        sudo yum install -y docker-ce docker-ce-cli containerd.io
        sudo systemctl start docker
        sudo systemctl enable docker
        sudo usermod -aG docker $USER
    fi
    
    # 安装Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        echo "安装Docker Compose..."
        sudo curl -L "https://github.com/docker/compose/releases/download/v2.21.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
        sudo chmod +x /usr/local/bin/docker-compose
    fi

else
    echo "不支持的操作系统: $OS"
    exit 1
fi

# 验证安装
echo ""
echo "=== 验证安装 ==="
echo "Go版本: $(go version 2>/dev/null || echo '未安装')"
echo "Node版本: $(node --version 2>/dev/null || echo '未安装')"
echo "Docker版本: $(docker --version 2>/dev/null || echo '未安装')"
echo "Docker Compose版本: $(docker-compose --version 2>/dev/null || echo '未安装')"

# 检查C库
echo ""
echo "检查C库依赖:"
pkg-config --exists jansson && echo "✓ jansson" || echo "✗ jansson"
pkg-config --exists liburcu && echo "✓ liburcu" || echo "✗ liburcu"
ldconfig -p | grep -q libnetfilter_queue && echo "✓ libnetfilter_queue" || echo "✗ libnetfilter_queue"

echo ""
echo "=== 依赖安装完成 ==="
echo "请重新登录以使Docker组权限生效，或运行: newgrp docker"