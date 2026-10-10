// 上游对接页：上游配置（含测试连接与余额）/ 下游推送（host/sync 登记信息）/ 上游主机（只读对账）。
import { useTranslation } from "react-i18next"

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

import { ConfigTab } from "./upstream/config-tab"
import { HostsTab } from "./upstream/hosts-tab"
import { PushTab } from "./upstream/push-tab"

export default function UpstreamPage() {
  const { t } = useTranslation()
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">{t("upstream.title")}</h1>
      <Tabs defaultValue="config">
        <TabsList>
          <TabsTrigger value="config">{t("upstream.tabs.config")}</TabsTrigger>
          <TabsTrigger value="push">{t("upstream.tabs.push")}</TabsTrigger>
          <TabsTrigger value="hosts">{t("upstream.tabs.hosts")}</TabsTrigger>
        </TabsList>
        <TabsContent value="config" className="mt-4">
          <ConfigTab />
        </TabsContent>
        <TabsContent value="push" className="mt-4">
          <PushTab />
        </TabsContent>
        <TabsContent value="hosts" className="mt-4">
          <HostsTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}
