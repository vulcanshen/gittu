# gittu — build / run / test / package（家族共用的 target 在 family.mk）
#
# 跑 `make`（或 `make help`）列出所有指令。

BINARY := gittu
include family.mk

##@ gittu 自己的

DEMO_REPO := $(CURDIR)/.local/demos/repo

.PHONY: demo-reset
demo-reset: ## 把 .local/demos/repo 重建回剛做好的樣子（分支、merge、tag、stash、會衝突的 clash 分支）
	@test -f .local/demos/mkdemo.sh || { echo "沒有 .local/demos/mkdemo.sh（.local/ 只在本機，不在 repo 裡）" >&2; exit 1; }
	sh .local/demos/mkdemo.sh "$(DEMO_REPO)"
