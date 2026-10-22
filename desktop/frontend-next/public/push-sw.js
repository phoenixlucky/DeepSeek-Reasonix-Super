const TEXT = {
  zh: {
    approval: ["有操作等你批准", "电脑上的 Reasonix 正停在那里等你。"],
    input: ["模型在等你回答", "电脑上的 Reasonix 需要你的回答才能继续。"],
    error: ["一轮因错误结束", "电脑上的 Reasonix 有一轮没有跑完。"],
    other: ["Reasonix", "电脑上有事等你。"],
  },
  en: {
    approval: ["An action needs your approval", "Reasonix on your computer is waiting for you."],
    input: ["The model is waiting for your answer", "Reasonix on your computer needs an answer to go on."],
    error: ["A turn ended with an error", "A turn on your computer did not finish."],
    other: ["Reasonix", "Something on your computer is waiting for you."],
  },
};

function describe(data) {
  const language = /^zh/i.test(self.navigator.language || "") ? "zh" : "en";
  const known = data && data.v === 1 && Object.hasOwn(TEXT.en, data.type) && data.type !== "other";
  const type = known ? data.type : "other";
  const count = known && Number.isInteger(data.count) && data.count > 1 && data.count < 100 ? data.count : 1;
  const [title, body] = TEXT[language][type];
  return { type, title: count > 1 ? `${title} (${count})` : title, body };
}

self.addEventListener("push", (event) => {
  let data = null;
  try {
    data = event.data ? event.data.json() : null;
  } catch {
    data = null;
  }
  const shown = describe(data);
  event.waitUntil(self.registration.showNotification(shown.title, {
    body: shown.body,
    tag: shown.type,
    renotify: true,
    icon: "icons/icon-192.png",
    badge: "icons/icon-192.png",
  }));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil((async () => {
    const open = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    const page = open.find((client) => client.url.startsWith(self.registration.scope));
    if (page) await page.focus();
    else await self.clients.openWindow(self.registration.scope);
  })());
});
