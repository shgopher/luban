
// 子应用 b：真实项目里这里会 import 一堆公共包（组件库/工具库/i18n）
(function () {
  var total = 0;
  for (var i = 0; i < 260; i++) total += SharedLib.fns['f' + i](i);
  var el = document.getElementById('b');
  el.innerHTML = '<button class="btn">b 的按钮</button><span class="sum">' + total + '</span>';
  window.__mounted = (window.__mounted || []).concat('b');
  if (window.__mounted.length === 2) performance.mark('content-ready');
})();
