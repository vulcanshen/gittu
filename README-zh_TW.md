# gitreu

[![Release](https://img.shields.io/github/v/release/vulcanshen/gitreu)](https://github.com/vulcanshen/gitreu/releases)
[![License](https://img.shields.io/github/license/vulcanshen/gitreu)](LICENSE)

**Language**: [English](README.md) · 繁體中文

**照 SourceTree 的畫面安排做的終端機 git 工具：左邊是分支，右邊是分支圖，能做的事都在 `Space` 選單裡。**

在 repo 裡打 `gitreu`，打開兩個面板：左邊是本地分支、遠端分支、tag 與 stash，右邊是 commit 與分支圖。SourceTree 右鍵選單上的動作，
都在 gitreu 的 `Space` 選單裡，對著 cursor 那個分支或 commit；工具列上的動作在 global operation 裡。看一個 commit 改了哪些檔、
每個檔的差異，比較兩個 commit，stage、commit、merge，衝突一塊一塊解、兩邊都看得到 —— 不用先背任何熱鍵。

![gitreu：分支圖、一個 commit 的檔案與差異、要 commit 的檔案、一次 merge 與解衝突](docs/demo.gif)

## 特色

- **SourceTree 的畫面安排**：左邊是本地分支、遠端分支、tag 與 stash，右邊是 commit 與分支圖；cursor 移到分支上，分支圖就跳到它的
  commit，按 `Enter` 就過去。
- **看得懂的分支圖**：一個 commit 一列，每條線一個顏色，分支與 tag 標在它們的 commit 上；可以看全部分支，或只看目前的分支。
- **先看再動手**：一個 commit 的檔案，再進去看每個檔的差異；標兩個 commit 一樣這樣比；`Esc` 一層一層退回。
- **所有操作都在 `Space` 選單**：checkout、merge、rebase、cherry-pick、revert、reset、開分支、打 tag、改名、刪除、stash、
  fetch、pull、push —— 需要先問的，執行前會先問。
- **Working tree**：衝突、已經 stage、還沒 stage 的檔案，旁邊是 cursor 那個檔的差異；stage、unstage、捨棄改動、commit。
- **衝突一塊一塊解**：我的在上、對方的在下，兩邊都看得到；選用我的、用對方的或兩個都要，最後一塊解完自動 stage。merge、
  rebase、cherry-pick、revert 做到一半時可以放棄或繼續。
- **用你自己的 git**：gitreu 呼叫你裝的 git，hooks、credential helper、ssh 金鑰與設定都跟在 shell 裡一樣。

## 安裝

### 需求

- macOS 或 Linux（WSL 也可以）。
- `PATH` 上有 git。
- 終端機用 [Nerd Font](https://www.nerdfonts.com)：圖示與框線都是 Nerd Font 的字。
- 支援 truecolor（24-bit）的終端機：256 色下分不出淡化的顏色與層色。
- 終端機至少 60 欄寬、32 列高；比這小時 gitreu 只顯示名稱、需要的大小與怎麼離開，視窗拉大後回到原本的樣子。

gitreu 啟動時會量 icon 與 powerline 符號各佔幾格。量錯時可以自己指定：

| 變數 | 指定什麼 |
|---|---|
| `GITREU__ICON_WIDTH` | icon 佔幾格，只給 gitreu |
| `GITREU__POWERLINE_WIDTH` | powerline 符號佔幾格，只給 gitreu |
| `TERMINU__ICON_WIDTH` | icon 佔幾格，terminu 家族每個 app 共用：設一次，每個 app 都讀得到 |
| `TERMINU__POWERLINE_WIDTH` | powerline 符號佔幾格，terminu 家族每個 app 共用 |

在家族 app 的終端機裡跑時，外層的 app 會替 gitreu 設好 `TERMINU__` 這兩個。

### Homebrew

```sh
brew install vulcanshen/tap/gitreu
```

### 安裝腳本

```sh
curl -fsSL https://raw.githubusercontent.com/vulcanshen/gitreu/main/install.sh | sh
```

不用 sudo：裝到 `~/.local/bin`。

### 第一次啟動

`gitreu` 打開目前目錄所在的 repo；`gitreu <路徑>` 打開那個路徑所在的 repo。不在 repo 裡時說一聲就結束。gitreu 自己不寫任何東西：
只有你執行操作時，才會改動你的 repo。

### 移除

```sh
brew uninstall gitreu
```

用安裝腳本裝的：

```sh
curl -fsSL https://raw.githubusercontent.com/vulcanshen/gitreu/main/uninstall.sh | sh
```

## 快速開始

```sh
cd your-repository
gitreu
```

用 `j`、`k` 移動，按 `Space` 看對 cursor 那個分支或 commit 能做什麼，在 commit 上按 `Enter` 看它改了哪些檔。`Tab` 換到另一個
面板，`W` 到 working tree，`H` 回到分支圖。`?` 列出所有按鍵，`q` 離開。

## 使用方式與按鍵

最下面一列一直寫著 `Space:menu ?:help !:log`：`Space` 列出在這裡能做的事，`?` 列出按鍵，`!` 打開 App Log，看 gitreu 做了什麼、
哪裡出錯。

### History

| 在哪裡 | `Enter` | `Space` 選單裡 |
|---|---|---|
| `[1]` BRANCHES：本地分支 | 到 `[2]` 它的 commit | `[c]`heck out、`[M]erge` 進目前的分支、把目前的分支 Re`[b]`ase 到它上面、`[r]`ename、`[D]`elete、Push |
| `[1]` REMOTES：遠端分支 | 到 `[2]` 它的 commit | `[c]`heck out 成追蹤它的本地分支、`[M]erge`、Re`[b]`ase、`[D]`elete 遠端的分支 |
| `[1]` TAGS：tag | 到 `[2]` 它的 commit | `[c]`heck out（HEAD 會 detached）、`[D]`elete、Push |
| `[1]` STASHES：stash | 看它存了什麼 | `[a]`pply、P`[o]`p、`[D]`rop |
| `[2]` commit | 它改了的檔案 | `[m]`ark、`[c]`heck out、`[b]`ranch here、`[t]`ag、`[M]`erge、R`[e]`base、Cherry-pick、Revert、Re`[s]`et 到這裡、Cop`[y]` hash |
| `[2]` 整個 panel | | Current br`[a]`nch／`[a]`ll branches、`[z]`oom、`[C]`ompare 標起來的兩個 commit、`[/]` 篩選 |

`[1]` 有 BRANCHES、REMOTES、TAGS、STASHES 四個分頁，用 `h`、`l` 切換；`[2]` 是 COMMITS，在上面按 `z` 放大：先放大到 `[1]` 只剩最小寬度，再到整個畫面，再按一次還原（終端機窄時直接到整個畫面）。upstream 在遠端被刪掉的分支，後面寫 `upstream gone`。在 commit 的檔案上按 `Enter` 打開它的
差異；`Esc` 回到檔案，再按一次回到分支圖。

### Working tree

| 在哪裡 | `Enter` | `Space` 選單裡 |
|---|---|---|
| 還沒 stage 的檔 | Stage | `[D]`iscard |
| 已經 stage 的檔 | Unstage | `[D]`iscard |
| 有衝突的檔 | 進去一塊一塊解 | 整個檔用 `[m]`ine、用 `[t]`heirs |
| 一塊衝突 | 到 `[2]` 讀完整的內容 | Use `[m]`ine、use `[t]`heirs、use `[b]`oth |
| 整個 panel | | Stage `[a]`ll、Unstage all |

`[1]` 是 CHANGES，`[2]` 是 DIFF：cursor 那個檔的差異；在那裡按 `Enter` 用 viewer 打開，可以找字、複製。

### 整個 repo 的動作

每個 `Space` 選單裡的 `Global operation`，列出 SourceTree 工具列上的動作：`[c]`ommit、Amend、`[p]`ull、`[P]`ush、`[f]`etch、
在 HEAD `[b]`ranch、`[M]`erge 一個分支、在 HEAD 打 `[t]`ag、`[S]`tash、Discard all、`[R]`efresh；merge、rebase、cherry-pick、
revert 做到一半時，還有 `[A]`bort 與 Continue。這些鍵在 panel 上也能直接按，panel 自己用了同一個鍵時以 panel 的為準：在 History 上 `c` 是 checkout，Commit 從 global operation 執行；在 `[1]` 上 `b` 是 rebase，Branch 從 global operation 執行。

gitreu 每 3 秒重新讀一次 repo，在外面改了東西，畫面也會跟著變。

## 疑難排解

- **push、pull 要密碼**：gitreu 沒辦法問密碼，git 會失敗並說明原因。請用 ssh agent，https 的話用 credential helper。
- **commit 被 hook 擋下**：hook 印的內容在錯誤訊息裡；再按一次 `[c]`，剛剛寫的訊息還在。

## 設定與資料

gitreu 沒有資料、狀態與快取。設定是 `$XDG_CONFIG_HOME/gitreu`（或 `~/.config/gitreu`；`GITREU__CONFIG` 指定別的目錄）裡的
`config.yaml`。gitreu 不會自己建它；沒有這個檔時用預設值。

```yaml
input_tab_width: 4   # commit 訊息裡按 Tab 插入幾個空白，差異裡的 Tab 也畫成這麼寬
```

這是 terminu 家族每個 app 共用的 key。讀不了的 `config.yaml`，gitreu 會問要不要移到 `config.yaml.bak`，不要就離開；值不是
大於 0 的整數時，原本的檔留在 `config.yaml.bak`，值改回 4。

## 限制

- 分支圖只讀最新的 1000 個 commit。
- gitreu 沒辦法問密碼或 passphrase：請用 ssh agent 或 credential helper。
- 衝突只顯示我的與對方的，不顯示共同的祖先。
- 沒有互動式 rebase、submodule、worktree、git-flow。
- 只有 macOS 與 Linux（WSL 也可以），沒有 Windows 版。

## 相關連結

- [CHANGELOG](CHANGELOG.md)

## terminu family

gitreu 遵循 terminu design：

- [terminu design](https://github.com/vulcanshen/terminu)：家族的設計規範
- [terminu-go](https://github.com/vulcanshen/terminu-go)：gitreu 用的 Go 函式庫
- [terminu-harness](https://github.com/vulcanshen/terminu-harness)：照 terminu design 做 app 的 Claude Code plugin

## License

[GPL-3.0](LICENSE)
