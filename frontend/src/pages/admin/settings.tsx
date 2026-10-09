// 系统设置页：短信设置 / 邮件设置 / 消息模板 三个标签页。
import { useTranslation } from "react-i18next"

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

import { EmailSettings } from "./settings/email-settings"
import { SmsSettings } from "./settings/sms-settings"
import { TemplatesSettings } from "./settings/templates"

export default function SettingsPage() {
  const { t } = useTranslation()
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">{t("admin.settingsTitle")}</h1>
      <Tabs defaultValue="sms">
        <TabsList>
          <TabsTrigger value="sms">{t("settings.tabSms")}</TabsTrigger>
          <TabsTrigger value="email">{t("settings.tabEmail")}</TabsTrigger>
          <TabsTrigger value="templates">{t("settings.tabTemplates")}</TabsTrigger>
        </TabsList>
        <TabsContent value="sms" className="mt-4">
          <SmsSettings />
        </TabsContent>
        <TabsContent value="email" className="mt-4">
          <EmailSettings />
        </TabsContent>
        <TabsContent value="templates" className="mt-4">
          <TemplatesSettings />
        </TabsContent>
      </Tabs>
    </div>
  )
}
