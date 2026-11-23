# 0.14-minitpl：最小「词法 → 语法」流水线

[0.14 文本处理范式](../../../v2/内功/0-内功/0.14-文本处理范式.md) 的配套代码：把 `a=1;b=hello;` 解析成键值对，Scanner 与 Parser 两段互不影响。

```bash
cd tools/examples/0.14-minitpl
go run .
```

示例输出：

```text
a      = 1 (int)
b      = "hello" (string)
port   = 8080 (int)
```

加一种新语法（如 `#` 注释）只改 `scan` 的 switch；加一种新结构（如数组）只改 `parse`——正文说的「两段互不影响」指的就是这条分界。
