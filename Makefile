# ruijia-tunlix Makefile
# 三个独立 Go module：proxy（公网代理端）、server（内网服务端）、client（本地客户端）
# 跨平台：支持 Windows、macOS、Linux

# 可执行文件扩展名：Windows 下为 .exe，macOS / Linux 为空
ifeq ($(OS),Windows_NT)
	EXT   := .exe
	SHELL := cmd.exe
else
	EXT   :=
	SHELL := /bin/sh
endif

# build_one：交叉编译单个「模块/平台」。按宿主系统选择设置环境变量的语法。
# 参数：$(1)=模块名 $(2)=GOOS $(3)=GOARCH $(4)=产物后缀(.exe 或空)
# 每次调用展开为「一条独立的 recipe 行」（前导 Tab + 结尾换行），
# 以便 Make 把每个平台的构建作为独立命令执行，避免 Windows cmd 不识别 ; 分隔符的问题。
ifeq ($(OS),Windows_NT)
define build_one
	cd $(1) && set "GOOS=$(2)" && set "GOARCH=$(3)" && $(GO) build -o ../$(BIN_DIR)/$(1)-$(2)-$(3)$(4) . && cd .. && echo built $(1) for $(2)/$(3)

endef
else
define build_one
	cd $(1) && GOOS=$(2) GOARCH=$(3) $(GO) build -o ../$(BIN_DIR)/$(1)-$(2)-$(3)$(4) . && cd .. && echo "built $(1) for $(2)/$(3)"

endef
endif

BIN_DIR := bin
GO      := go
MODULES := proxy server client

# 交叉编译目标平台（GOOS/GOARCH 组合）
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

.PHONY: all build proxy server client test vet fmt tidy clean run-proxy run-server run-client cross help

all: build ## 默认目标：编译全部模块

build: proxy server client ## 编译 proxy / server / client 三个模块

proxy: ## 编译代理端
	cd proxy && $(GO) build -o ../$(BIN_DIR)/proxy$(EXT) .

server: ## 编译服务端
	cd server && $(GO) build -o ../$(BIN_DIR)/server$(EXT) .

client: ## 编译客户端
	cd client && $(GO) build -o ../$(BIN_DIR)/client$(EXT) .

# cross：交叉编译全部模块到全部目标平台，产物命名为 <模块>-<os>-<arch>[.exe]
# Windows / macOS / Linux 均可运行（已按宿主系统自动选择环境变量设置语法）。
cross: ## 交叉编译各平台二进制（Windows/macOS/Linux 均可）
	$(foreach p,$(PLATFORMS),$(foreach m,$(MODULES),$(call build_one,$(m),$(word 1,$(subst /, ,$(p))),$(word 2,$(subst /, ,$(p))),$(if $(findstring windows,$(p)),.exe,))))

test: ## 运行全部单元测试
	cd client && $(GO) test ./...

vet: ## 对三个模块执行 go vet
	cd proxy  && $(GO) vet ./...
	cd server && $(GO) vet ./...
	cd client && $(GO) vet ./...

fmt: ## 格式化全部模块代码
	cd proxy  && $(GO) fmt ./...
	cd server && $(GO) fmt ./...
	cd client && $(GO) fmt ./...

tidy: ## 整理三个模块的依赖
	cd proxy  && $(GO) mod tidy
	cd server && $(GO) mod tidy
	cd client && $(GO) mod tidy

run-proxy: proxy ## 编译并启动代理端
	cd proxy && ../$(BIN_DIR)/proxy$(EXT)

run-server: server ## 编译并启动服务端
	cd server && ../$(BIN_DIR)/server$(EXT)

run-client: client ## 编译并启动客户端
	cd client && ../$(BIN_DIR)/client$(EXT)

clean: ## 清理编译产物
ifeq ($(OS),Windows_NT)
	if exist $(BIN_DIR) rmdir /s /q $(BIN_DIR)
else
	rm -rf $(BIN_DIR)
endif

help: ## 显示可用命令
	@echo "ruijia-tunlix make targets:"
	@echo "  make build                 - 编译全部模块"
	@echo "  make proxy/server/client   - 单独编译某个模块"
	@echo "  make cross                 - 交叉编译各平台二进制(Windows/macOS/Linux)"
	@echo "  make test                  - 运行单元测试"
	@echo "  make vet                   - 静态检查"
	@echo "  make fmt                   - 格式化代码"
	@echo "  make tidy                  - 整理依赖"
	@echo "  make run-proxy             - 启动代理端"
	@echo "  make run-server            - 启动服务端"
	@echo "  make run-client            - 启动客户端"
	@echo "  make clean                 - 清理产物"