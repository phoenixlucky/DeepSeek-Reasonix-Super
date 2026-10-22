import { t } from "../i18n";
import type { HubPort } from "../port/hub";
import type { AgentPort } from "../port/port";
import { Group } from "./Group";
import { PeerMessages } from "./PeerMessages";
import { PeerProjects } from "./PeerProjects";
import { usePeersSettings } from "./usePeersSettings";

// Messages between the user's own sessions: whether they are on, and which projects
// take one without a card.
export function PeersSection({ port, hub }: { port: AgentPort; hub: Pick<HubPort, "tree"> }) {
  const settings = usePeersSettings(port);
  return (
    <>
      <Group id="peers"
        title={t("会话间消息（实验）")}
        hint={t("同一台机器上的会话互相发文字。收到的文字先挂起，只有你批准才会交给模型；发出去的文字也要对方的用户批准。没有给模型用的工具，默认关闭。")}
      >
        <PeerMessages port={port} settings={settings} />
      </Group>
      <Group id="peer-projects"
        title={t("放行的项目")}
        hint={t("这些项目里的会话收到你其他会话的消息时不再询问，直接交给模型。只写在你自己的用户配置里，项目配置改不了；随时可以撤销。")}
      >
        <PeerProjects port={port} hub={hub} settings={settings} />
      </Group>
    </>
  );
}
