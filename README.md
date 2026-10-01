# 三角洲地图文件备份与恢复

一个 Windows 小工具，用来**临时把《三角洲行动》里的两个大地图文件挪到备份文件夹里**，需要时再一键还原。

> 一句话：点一下「备份」，把占地方的两个地图文件暂时收起来；点一下「恢复」，再放回去。

---

## 这个软件能做什么

它只搬动下面这 **2 个** 文件（都在游戏的 `PackContent\Paks` 目录里）：

- `pak-0-2-pakchunk90-WindowsClient.pak`（约 9.1 GB）
- `pak-0-2-pakchunk90optional-WindowsClient.pak`（约 2.6 GB）

- **备份**：把这 2 个文件从 `Paks` 目录**剪切**到 `Paks\MapBackup` 文件夹。
- **恢复**：把这 2 个文件从 `Paks\MapBackup` 文件夹**剪回** `Paks` 目录。

除了这 2 个文件和 `MapBackup` 文件夹，程序**不会**创建、移动或删除任何其它东西。

---

## 重要警告（务必先读）

1. **备份后，游戏里对应的地图将无法加载**，需要点「恢复」才能还原。
2. **操作前请完全退出游戏和 WeGame。** 如果检测到游戏或反作弊（ACE 等）还在运行，程序会拒绝操作。
3. 备份的本质是**在同一块磁盘内改名 / 移动**，几乎瞬间完成，**不会额外占用磁盘空间**（所以不需要准备额外的空余容量）。

---

## 怎么用（小白向）

1. 打开 GitHub 仓库的 **Releases** 页面，下载 `DeltaForceMapBackup.exe`。
2. **双击**运行（可能出现杀软误报，见下方「常见问题」）。
3. 程序会自动找游戏目录。界面大致长这样：

   - **顶部**：显示游戏安装路径，右边有「自动查找」和「手动指定…」两个按钮。
   - **左边卡片「备份」**：点「开始备份」。
   - **右边卡片「恢复」**：点「开始恢复」。
   - **底部「操作记录」**：显示每一步做了什么。

4. 点按钮后会弹出确认框，确认后才真正开始。

### 备份步骤

1. 确认顶部路径正确（不对就点「手动指定…」选择 DeltaForce 文件夹）。
2. 点左边「开始备份」。
3. 在确认框里点「确定」。
4. 完成后，底部会显示「已移动：…」。

### 恢复步骤

1. 点右边「开始恢复」。
2. 在确认框里点「确定」。
3. 完成后，底部会显示「已移动：…」，游戏内地图即可正常加载。

---

## 备份文件夹在哪？

在游戏的 `Paks` 目录下，名字叫 `MapBackup`。例如：

```
E:\WeGameApps\rail_apps\DeltaForce(2001918)\DeltaForce\PackContent\Paks\MapBackup\
```

---

## 常见问题

**Q：提示「未找到游戏目录」怎么办？**
A：点「手动指定…」，选择**包含 `PackContent` 的那一级 DeltaForce 文件夹**（例如 `…\DeltaForce(2001918)\DeltaForce`），或者选择该文件夹的上一层，程序会自动识别。

**Q：杀毒软件 / 安全软件误报怎么办？**
A：这是自研的绿色小工具，没有恶意代码。若被拦截，请把该 exe 加入杀软白名单 / 信任列表后再运行。

**Q：备份会不会占很多空间？**
A：不会。备份是在同一块磁盘内移动文件，不会复制出额外副本，几乎不占额外空间。

**Q：点了备份，游戏地图进不去了？**
A：这是正常现象，说明备份成功了。点「恢复」即可还原。

---

## 开发者信息

- 技术栈：Go 1.24 + [Fyne v2](https://fyne.io/)（GUI）。
- 目标平台：仅 Windows amd64。
- 交付两个可执行文件：
  - `DeltaForceMapBackup.exe` — 图形界面。
  - `dfmap-cli.exe` — 命令行版（供自动化测试 / 脚本调用，输出单行 JSON）。

### 如何构建

**本机不需要安装 Go**。把代码推到 GitHub 仓库后，GitHub Actions（`.github/workflows/build.yml`）会在 Windows 运行器上自动：

1. 安装 Go 1.24、准备 gcc（CGO 需要）。
2. `go get` / `go mod tidy` 解析依赖。
3. `go vet` 静态检查。
4. 用 `rsrc` 生成图标 + 清单资源。
5. 编译出 `DeltaForceMapBackup.exe` 与 `dfmap-cli.exe`。

构建产物：
- 每次推送：在 Actions 页面下载 **Artifact**（`DeltaForceMapBackup-windows-amd64`）。
- 打 tag（形如 `v1.0.0`）推送：自动创建 **Release** 并附带两个 exe。

### 命令行用法

```
dfmap-cli.exe <find|status|backup|restore> [flags]

  --path DIR          指定游戏目录
  --config FILE       指定配置文件路径
  --conflict MODE     冲突策略：ask|overwrite|skip（CLI 下 ask 等价于 skip）
  --dry-run           只报告，不真正移动
  --json              以 JSON 输出（默认即为 JSON）
```

stdout 只输出一行 JSON，退出码：成功 `0`，失败 `1`。
