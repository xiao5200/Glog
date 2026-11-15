# Third-Party Notices / 第三方组件声明

GLog's original source code is licensed under the Apache License 2.0 (see `LICENSE`).

**Windows binary distribution:** the released `GLog.exe` statically links the following GPL-3.0 components:

- `github.com/0xrawsec/golang-evtx` v1.2.9 — https://github.com/0xrawsec/golang-evtx
- `github.com/0xrawsec/golang-utils` v1.3.0 — https://github.com/0xrawsec/golang-utils

Because the combined binary includes GPL-3.0 code, **the combined `GLog.exe` binary is distributed under the GNU General Public License v3.0**. The complete corresponding source is this repository (https://github.com/xiao5200/Glog) together with the dependency versions pinned in `go.mod` / `go.sum`. The full GPL-3.0 text is provided in `LICENSE-GPL-3.0`.

The original GLog source files remain available under Apache-2.0; Apache-2.0 is compatible with GPL-3.0 for inclusion in the combined work.

## Direct dependencies / 直接依赖

| Module | Version | License |
|---|---|---|
| github.com/0xrawsec/golang-evtx | v1.2.9 | GPL-3.0 |
| github.com/0xrawsec/golang-utils | v1.3.0 | GPL-3.0 |
| github.com/BurntSushi/toml | v1.6.0 | MIT |
| github.com/RoaringBitmap/roaring | v1.9.4 | Apache-2.0 |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/pierrec/lz4/v4 | v4.1.33 | BSD-3-Clause |
| github.com/pkg/sftp | v1.13.12 | BSD-2-Clause |
| github.com/wailsapp/wails/v2 | v2.17.1 | MIT |
| go.uber.org/zap | v1.28.0 | MIT |
| golang.org/x/crypto | v0.57.0 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | Apache-2.0 |
| modernc.org/sqlite | v1.62.0 | BSD-3-Clause |

Transitive dependencies and their exact versions are listed in `go.mod` and `go.sum`; each upstream project retains its own license. Frontend npm dependencies are listed in `frontend/package-lock.json` and retain their respective licenses.

## 中文说明

GLog 原创源代码采用 Apache License 2.0（见 `LICENSE`）。Windows 可执行文件静态链接了 GPL-3.0 组件 `golang-evtx` 与 `golang-utils`，因此**合并后的 `GLog.exe` 二进制按 GNU GPL v3.0 分发**。对应完整源代码即本仓库（配合 `go.mod`/`go.sum` 锁定的依赖版本），GPL-3.0 全文见 `LICENSE-GPL-3.0`。原始 GLog 源代码仍可按 Apache-2.0 使用；Apache-2.0 与 GPL-3.0 在合并作品中单向兼容。
