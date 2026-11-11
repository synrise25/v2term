# 使用指南

[返回项目首页](../README.md)

## 导航与阅读

启动参数放在节点名或帖子地址之前。没有阅读记录时默认打开「最热」，已有记录时恢复上次退出的页面。按 `g` 可以输入节点名、帖子编号、站内路径或 `https://www.v2ex.com` 地址。

| 操作 | 按键或鼠标 |
| --- | --- |
| 保存退出 | `q`；编辑回复时用 `Ctrl+C` |
| 选择帖子、主题作者或楼层 | `↑` `↓` / `k` `j` / `Tab` `Shift+Tab` |
| 打开帖子 | `Enter` / `→` / 鼠标右键；左键打开点击的帖子 |
| 返回上一页 | `h` / `Backspace` / `Alt+←` |
| 滚动 / 翻屏 | 滚轮 / `PgUp` `PgDn` / `Ctrl+U` `Ctrl+D` |
| 最热 / 最新 / 问与答 / 技术 | `1` / `2` / `3` / `4` |
| 打开节点或地址 | `g` |
| 当前列表筛选 / 正文查找 | `/`，`Esc` 清除 |
| 上一页 / 下一页 | `[` `]` / `p` `n` |
| 切换对话、时间顺序 | `v` |
| 强制刷新 / 帮助 | `R` / `?` |
| 展开、收起图片 | 点击图片按钮或已展开的图片 |
| 打开正文链接、原图 | 使用终端的链接打开手势，如 macOS 的 Command 点击 |

顶部分类、底部操作栏和回复旁的按钮均可点击。常见终端可按住 `Shift` 使用原生文字选择。

在首条内容按 `↑` 进入分类栏，用 `←` `→` 选择，`Enter` 确认，`↓` 回到内容。列表内容获得焦点时，`←` 与 `q` 一样保存退出。

帖子中，`→` 依次选择当前楼层的「回复」「感谢」，`Enter` 执行；`←` 逐级退出操作焦点，再返回上一页。`Esc` 取消操作焦点。

按 `q` 正常退出时保存当前页、滚动位置、选中项、查找词、最多 50 步导航历史、草稿和视图偏好。`--fresh` 忽略旧状态，从「最热」开始，退出时写入本次状态；程序异常终止时不保证恢复最新进度。图片重新进入页面时保持折叠。

## 对话与热评

回复默认按对话排列，始终保留原楼层号。优先关联明确引用的前文楼层；只有 @ 用户时，关联该用户最近的一条前文，并用 `≈` 标记推断。关联只使用当前分页，无法匹配的回复独立显示。

`├─` 表示后面还有同级回复，`└─` 表示同级最后一条；缩进达到上限后用 `↪ #楼层` 保留父回复指向。按 `v` 切换时间顺序，热评区仍按对话显示。

正文下默认展示当前页感谢数至少为 3 的前 3 条回复，按感谢数排序，并带上后续对话。重叠分支合并，原位置的回复仍然保留。没有符合条件的回复时隐藏热评区。

```sh
./v2term --hot-replies 5 --hot-min-thanks 10 1246944
./v2term --hot-replies 0                # 本次关闭热评
./v2term config set hot_replies 5       # 保存偏好
./v2term config set hot_min_thanks 10   # 包含恰好 10 次感谢的回复
```

## 登录、回复与感谢

在浏览器登录 `https://www.v2ex.com`，从开发者工具的网络面板找到同域请求，复制 **Cookie 请求头的值**，保存为只有一行的本地文件，再导入：

```sh
chmod 600 /path/to/v2ex-cookie
./v2term login --cookie-file /path/to/v2ex-cookie
./v2term                               # 自动使用已保存的会话
./v2term logout                        # 删除本地保存的 Cookie
```

Cookie 等同登录凭据，不要粘贴到 Issue、截图或代码仓库。`login --cookie-file -` 支持从标准输入导入；`logout` 不会注销浏览器会话。

一次性使用可传 `--cookie-file 文件` 或设置 `V2TERM_COOKIE`。优先级为命令行文件、环境变量、已保存文件；临时来源不会自动导入或改写。

| 操作 | 按键 |
| --- | --- |
| 回复所选楼层或主题 | `r` |
| 始终回复主题 | `a` |
| 感谢所选楼层或主题 | `t` |
| 始终感谢主题作者 | `T` |
| 发送回复 / 保留草稿返回 | `Ctrl+S` / `Esc` |

回复楼层自动填写 `@用户名 #楼层`，不同回复对象分别保留草稿。编辑时普通字母均作为正文输入，`Ctrl+C` 保存退出。

感谢需要确认，并按网站规则扣除铜币。回复和感谢不自动重试；如果网络异常导致结果不确定，先刷新核对，再决定是否重发。

## 图片与链接

图片默认折叠，点击「显示图片」才加载。正文链接和图片按钮使用终端原生超链接，程序不主动启动浏览器。

| 终端 | 自动选择的图片协议 |
| --- | --- |
| Warp、Kitty、Ghostty | Kitty |
| iTerm2、WezTerm | iTerm2 |
| macOS 原生 Terminal 及其他终端 | 彩色字符预览 |

实际兼容性受终端版本及 tmux、SSH 等转发环境影响，显示异常时可切换协议：

```sh
./v2term --image-protocol blocks        # 字符预览
./v2term --image-protocol kitty
./v2term --image-protocol iterm2
./v2term config set image_protocol auto
```

支持 PNG、JPEG、GIF 首帧和 WebP。单张最多读取 12 MiB、解码 2000 万像素，最多并发加载两张；预览最多 72 列、16 行。图片仅在内存缓存，切换页面清空，请求不携带 V2EX Cookie。

## 配置与本地数据

首次启动不需要创建配置文件。用 `config show` 查看当前配置、实际路径和 Cookie 来源，不会显示 Cookie 内容。

```sh
./v2term config show
./v2term config set scroll_lines 1
./v2term config set cache_ttl 5m
./v2term --scroll-lines 2 --cache-ttl 5m go   # 仅覆盖本次运行
./v2term --config /path/to/config.json config show
```

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `scroll_lines` | `1` | 每次滚轮事件移动 1–20 行 |
| `cache_ttl` | `1m` | 页面缓存有效期，0 至 24 小时 |
| `cache_size` | `32` | 内存缓存页数，0–1000；0 关闭 |
| `image_protocol` | `auto` | `auto`、`kitty`、`iterm2` 或 `blocks` |
| `hot_replies` | `3` | 热评数量，0–100；0 关闭 |
| `hot_min_thanks` | `3` | 非负整数，最低感谢数，包含该值 |
| `state_dir` | 见下表 | 阅读进度和草稿目录 |
| `cookie_file` | 配置文件同目录的 `cookies.txt` | 已保存的登录会话 |

Linux 默认路径如下，其他环境以 `config show` 为准：

| 数据 | 默认位置 |
| --- | --- |
| 配置 | `~/.config/v2term/config.json`，遵循 `XDG_CONFIG_HOME` |
| Cookie | `~/.config/v2term/cookies.txt` |
| 阅读进度与草稿 | `~/.local/state/v2term/state.json`，遵循 `XDG_STATE_HOME` |
| 页面与图片缓存 | 仅内存，退出清空 |

配置和持久化文件以 `0600` 权限写入。正常退出时，已导入会话会保存服务器更新后的 Cookie；阅读状态不含 Cookie 或请求令牌。

`R` 强制刷新；成功回复或感谢后清空页面缓存。`--no-cache` 仅关闭本次运行的页面缓存。
