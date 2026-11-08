(function () {
  var me = document.currentScript, zone = me.getAttribute("data-zone") || undefined;
  var el = me.parentNode.querySelector(".dxop-clock [data-time]") || document.querySelector(".dxop-clock [data-time]");
  function tick() { el.textContent = new Date().toLocaleTimeString("en-GB", { timeZone: zone }); }
  tick(); setInterval(tick, 1000);
})();
