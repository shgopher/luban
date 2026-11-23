
// —— 以下到 COMPONENT_END 为止，CSR 与 SSR 用的是同一份字符串 ——
function ItemList(props) {
  return React.createElement('ul', { className: 'items' },
    props.items.map(function (it) {
      return React.createElement('li', { key: it.id },
        React.createElement('h2', null, it.title),
        React.createElement('p', null, it.summary),
        React.createElement('span', null, it.author));
    }));
}
function App(props) {
  return React.createElement('main', null,
    React.createElement('h1', null, '条目列表'),
    React.createElement('p', { className: 'meta' }, '共 ' + props.items.length + ' 条'),
    React.createElement(ItemList, { items: props.items }));
}
var ProbeEffect = function () {
  // 水合完成的信号：effect 只在客户端跑，服务端渲染不出它
  React.useEffect(function () { performance.mark('hydrated'); }, []);
  return null;
};
// —— COMPONENT_END ——

// ① 先取数据（真实 SPA 的常态：HTML 到了才有 JS，JS 到了才发请求）
fetch('/api/items').then(function (r) { return r.json(); }).then(function (d) {
  var root = ReactDOM.createRoot(document.getElementById('root'));
  // ② flushSync 保证 DOM 已提交后再打点，否则测到的是"提交前"
  ReactDOM.flushSync(function () { root.render(React.createElement(App, { items: d.items })); });
  performance.mark('content-ready');
  performance.mark('interactive');
});
