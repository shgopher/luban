# 四个「未验证」案例真实执行记录（verify-npm）

- 执行时间：2026-11-22 16:24–16:33 CST（Asia/Shanghai）
- 主机：macOS arm64；node v26.10.0；npm 11.19.1；pnpm 8.5.1；yarn 1.22.22
- 所有工程在 /tmp/luban-verify/work-npm/，所有原始日志在 /tmp/luban-verify/out/
- 未修改 /Users/shgopher/Desktop/family/luban 下任何文件（四个源文件 mtime 均早于本次会话）

## 结论总览

| # | 案例 | 结论 | 关键数字 |
|---|---|---|---|
| 1 | orval 生成 + 重生成 diff 为空（73.4，约 275 行） | ✅ 通过 | orval 8.48.0；两次生成 diff -r 退出码 0；单文件 sha256 一致；split 模式 2 文件 sha256 一致；改 spec 后门禁退出码 1 |
| 2 | monorepo 只跑受影响包（51.4，约 305 行） | ✅ 通过 | pnpm 8.5.1；全量 3 包；改 pkg-b → 1 包；改 pkg-a → 2 包（pkg-a+pkg-b，pkg-c 被跳过）；故意改坏 → exit 1 |
| 3 | Promptfoo 自定义 provider 评测（64.9，约 239 行） | ✅ 通过（需两处环境规避） | promptfoo 0.126.0；2 case 2 passed / 0 failed，exit 0；must-fail 1 passed / 1 failed，exit 100；promptfoo view HTTP 200 |
| 4 | uni-app / KMP 版本核实（73.3，236–237 行） | ✅ 查到真实数据 | uni-app latest 2.0.2-5030320261023001（2026-10-23）；Kotlin/KMP 稳定版 2.4.21（2026-10-08）；Compose Multiplatform 稳定版 1.12.2（2026-10-15） |

## 环境规避（必须写进方法，否则别人复现会卡住）

1. `npm i` 首次失败：`~/.npm/_cacache` 存在 root 属主文件，报 `EPERM ... /Users/shgopher/.npm/_cacache/tmp/***`。
   规避：全程 `export npm_config_cache=/tmp/luban-verify/npm-cache`（不改本机 ~/.npm 权限）。
2. promptfoo CLI 首次运行失败：`ENOENT: no such file or directory, open '/Users/shgopher/.promptfoo/promptfoo.yaml'`（该目录不存在）。
   规避：`export PROMPTFOO_CONFIG_DIR=/tmp/luban-verify/promptfoo-home && PROMPTFOO_DISABLE_TELEMETRY=1`。
3. promptfoo 用 `npm i --ignore-scripts promptfoo`（577 个包，3 分钟），避免 postinstall；eval 与 view 均正常。
4. orval 未加 `--ignore-scripts`；npm 11 默认拦截 esbuild 的 postinstall（日志有 `npm warn install-scripts esbuild@0.28.2`），orval 生成不受影响。

## 任务 1：orval（73.4 前后端契约）

- orval 版本：`npx orval --version` → `8.48.0`；`node_modules/orval/package.json` → `8.48.0`
- spec：OpenAPI 3.0.3，2 条路径（GET/POST /refunds），2 个 schema（Refund/RefundInput）
- 配置：`orval.config.ts`（书里写的就是 .ts，实测可加载），`client: 'axios'`, `mode: 'single'`, `clean: true`
- 两次生成完全相同：
  - `npx --yes orval --config orval.config.ts` ×2，均 exit 0，输出 `🍻 orval v8.48.0`
  - `diff -r orval-run1 orval-run2` → `DIFF_EXIT=0`
  - 单文件 100 行；sha256 两次均为 `050d46af…8e4d9a`
- split 多文件模式（refund.ts + refund.schemas.ts，共 115 行）两跑：`diff -r` exit 0，两文件 sha256 两次一致。
- 门禁语义（书里 `git diff --exit-code -- src/api/generated`）：
  - 重新生成后 `GATE_EXIT=0`
  - 手工加一行漂移 → `GATE_EXIT_AFTER_DRIFT=1`
  - 改 spec（Refund 增加 refundReason 字段）后重生成 → diff 1 行新增，`GATE_EXIT_AFTER_SPEC_CHANGE=1`
- 日志：
  - /tmp/luban-verify/out/73.4-orval-install.log
  - /tmp/luban-verify/out/73.4-orval-version.log
  - /tmp/luban-verify/out/73.4-orval.log
  - /tmp/luban-verify/out/73.4-orval-split.log

## 任务 2：pnpm workspace 受影响包（51.4 单仓 vs 多仓）

- pnpm 8.5.1；workspace：pkg-a、pkg-b（`dependencies: pkg-a: workspace:*`）、pkg-c（独立）
- 每个包 `test` 跑 `node test.js`，`node:assert/strict` 真断言（pkg-a 4 条、pkg-b 3 条、pkg-c 2 条）
- 全量 `pnpm -r test`：`Scope: 3 of 4 workspace projects`，3 包全过，exit 0
- 改 pkg-b（加 sumSquares）后：
  - `pnpm --filter "...[HEAD]" test` → 只跑 pkg-b 1 个包，exit 0
  - 选择器核对 `pnpm --filter "...[HEAD]" exec pwd` → 仅 packages/pkg-b
- 提交后再改 pkg-a（sum 改写 + 加 product）：
  - `pnpm --filter "...[HEAD]" test` → `Scope: 2 of 4`，pkg-a + pkg-b，pkg-c 未跑，exit 0
  - `exec pwd` → packages/pkg-a、packages/pkg-b
- 负向对照：把 pkg-a 的 `sum` 改成 `a - b` → `pnpm --filter "...[HEAD]" test` 报
  `AssertionError [ERR_ASSERTION]: Expected values to be strictly equal: -1 !== 5`，
  `ERR_PNPM_RECURSIVE_RUN_FIRST_FAIL`，退出码 1；`git checkout` 还原后恢复 exit 0
- 无改动时 `...[HEAD]` 一个包都不选：`No projects matched the filters`（exit 0）——CI 里要显式处理
- 选择器语义（pnpm 官方文档 https://pnpm.io/filtering 抓取 + 本机实测一致）：
  `foo...` = foo+依赖；`...foo` = foo+依赖者；`foo^...` = 仅依赖；`...^foo` = 仅依赖者
  （实测 `pkg-b...` 选中 pkg-a+pkg-b；`...pkg-b` 只选 pkg-b；`...pkg-a` 选中 pkg-a+pkg-b）
- 日志：
  - /tmp/luban-verify/out/51.4-pnpm-install.log
  - /tmp/luban-verify/out/51.4-pnpm-recursive.log
  - /tmp/luban-verify/out/51.4-pnpm-affected.log
  - /tmp/luban-verify/out/51.4-pnpm-affected2.log
  - /tmp/luban-verify/out/51.4-pnpm-mustfail.log
  - /tmp/luban-verify/out/51.4-pnpm-static-selector.log

## 任务 3：Promptfoo（64.9 评测与护栏）

- promptfoo 0.126.0（`npx promptfoo --version`）；本地安装 577 包
- 自定义 provider：`provider.js`（CJS class，`callApi` 固定返回 `no — refunds are only allowed within 7 days.`，metadata 带 question）
- 2 case 全过：
  - `promptfoo eval -c promptfooconfig.yaml --no-cache --no-progress-bar`
  - `✓ 2 passed (100%) / 0 failed (0%) / 0 errors (0%)`，`EVAL_EXIT=0`
  - JSON stats：successes 2, failures 0, errors 0, numRequests 2
- must-fail 真实失败：
  - `promptfoo eval -c promptfooconfig-mustfail.yaml` → `[FAIL]` 行，`MUSTFAIL_EXIT=100`
  - stats：successes 1, failures 1；失败原因 `Expected output to contain "yes"`
- `promptfoo view -n -p 15555`：13s 就绪，`HTTP/1.1 200 OK`，`<title>promptfoo</title>`，`Server running at http://localhost:15555`，结束后 shutdown
- 日志：
  - /tmp/luban-verify/out/64.9-promptfoo-install.log
  - /tmp/luban-verify/out/64.9-promptfoo-version.log
  - /tmp/luban-verify/out/64.9-promptfoo.log
  - /tmp/luban-verify/out/64.9-promptfoo-eval.json
  - /tmp/luban-verify/out/64.9-promptfoo-mustfail.json
  - /tmp/luban-verify/out/64.9-promptfoo-view-server.log

## 任务 4：uni-app / KMP 版本（73.3 表格）

### uni-app（npm registry，查询时间 2026-11-22 16:31 CST）

- `npm view @dcloudio/uni-app version` → `2.0.2-5030320261023001`
- dist-tags：
  - latest = `2.0.2-5030320261023001`
  - next = `2.0.2-5030420261028001`
  - vue3 = `3.0.0-alpha-5030720261117001`（alpha，不是稳定版）
  - alpha = `2.0.2-alpha-5030720261117001`
  - nvue3 = `3.0.0-alpha-3040320220324001`
- latest 发布时间（time json）：`2026-10-23T08:45:27.354Z`；tarball HTTP 200，Last-Modified `Fri, 23 Oct 2026 08:45:31 GMT`
- 来源：https://registry.npmjs.org/@dcloudio/uni-app

### Kotlin Multiplatform（Maven Central）

- `repo1.maven.org/.../kotlin-gradle-plugin/maven-metadata.xml`：`<latest>2.5.0-RC</latest>`（预发布）、`<release>2.5.0-RC</release>`，`lastUpdated=20261111082538`
- 稳定版（过滤 -RC/-Beta/-Alpha/-M）：最高 `2.4.21`；2.4.21 pom/jar HTTP 200，Last-Modified `Thu, 08 Oct 2026 07:57:52/53 GMT`，jar 30,714,368 字节
- KMP Gradle 插件 marker `org.jetbrains.kotlin.multiplatform.gradle.plugin` 同样到 `2.4.21`（marker pom HTTP 200，2026-10-08）
- `kotlin-stdlib`：latest/2.4.21 同上（pom 2026-10-08 07:57:54）
- Compose Multiplatform `compose-gradle-plugin`：`<latest>1.13.0-rc01</latest>`（预发布），稳定版最高 `1.12.2`（pom 2026-10-15 10:16:24，jar 3,922,215 字节）
- 注意：search.maven.org 的 solrsearch 索引滞后，`latestVersion` 仍返回 `2.2.0`（timestamp 1750659500357）；与 maven-metadata.xml 不一致，写书时不要引用 solr 的 latestVersion
- 来源：
  - https://search.maven.org/solrsearch/select?q=g:org.jetbrains.kotlin+AND+a:kotlin-gradle-plugin&rows=1&wt=json
  - https://repo1.maven.org/maven2/org/jetbrains/kotlin/kotlin-gradle-plugin/maven-metadata.xml
  - https://repo1.maven.org/maven2/org/jetbrains/kotlin/multiplatform/org.jetbrains.kotlin.multiplatform.gradle.plugin/maven-metadata.xml
  - https://repo1.maven.org/maven2/org/jetbrains/compose/compose-gradle-plugin/maven-metadata.xml
- 日志：
  - /tmp/luban-verify/out/73.3-uniapp-npm.log
  - /tmp/luban-verify/out/73.3-uniapp-detail.log
  - /tmp/luban-verify/out/73.3-kmp-maven.log
  - /tmp/luban-verify/out/73.3-kmp-metadata.log
  - /tmp/luban-verify/out/73.3-kmp-metadata-detail.log
  - /tmp/luban-verify/out/73.3-kmp-plugin-marker.log
  - /tmp/luban-verify/out/73.3-kmp-stable-filter.log

## 可以写进书里的措辞（标注数据来源）

1. 【本次实测】orval 8.48.0 本机实测：以 OpenAPI 3.0.3 生成 axios 客户端，连续两次生成后 `diff -r` 退出码 0、文件 sha256 一致；改 spec 增加一个字段后 `git diff --exit-code -- src/api/generated` 退出码 1（单文件 100 行；split 模式 2 文件 115 行）。
2. 【本次实测】pnpm 8.5.1：3 包 workspace 全量 `pnpm -r test` 跑 3 个包；改 pkg-b 后 `pnpm --filter "...[HEAD]" test` 只跑 1 个包；改 pkg-a 后跑 2 个包（pkg-a 与被依赖它的 pkg-b），独立包 pkg-c 被跳过；故意改坏断言时命令退出码 1 并输出 `ERR_PNPM_RECURSIVE_RUN_FIRST_FAIL`。
3. 【本次实测】Promptfoo 0.126.0：自定义 JS provider（固定输出、零 API key）2 case 评测 2 passed / 0 failed，退出码 0；故意让断言失败时 1 passed / 1 failed，退出码 100；`promptfoo view -n -p 15555` 返回 HTTP 200。
4. 【注册源查询，2026-11-22】uni-app npm latest = 2.0.2-5030320261023001（2026-10-23 发布），vue3 标签为 3.0.0-alpha-5030720261117001（alpha）。
5. 【注册源查询，2026-11-22】Kotlin/KMP 稳定版 2.4.21（Maven Central，2026-10-08 发布；latest 标记 2.5.0-RC 为预发布）；Compose Multiplatform 稳定版 1.12.2（2026-10-15 发布）。Maven Central 的 solrsearch latestVersion 存在滞后（返回 2.2.0），应以 maven-metadata.xml 为准。
