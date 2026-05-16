import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import { RequestRulesTable } from './components/request-rules-table'
import { RequestLogsTable } from './components/request-logs-table'
import { RuleFormDialog } from './components/rule-form-dialog'
import type { RequestRule } from './types'

export function RequestRules() {
  const { t } = useTranslation()
  // 控制规则表单对话框
  const [formOpen, setFormOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<RequestRule | null>(null)

  const handleCreate = () => {
    setEditingRule(null)
    setFormOpen(true)
  }

  const handleEdit = (rule: RequestRule) => {
    setEditingRule(rule)
    setFormOpen(true)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Request Rules')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Tabs defaultValue='rules' className='w-full'>
          <TabsList>
            <TabsTrigger value='rules'>{t('Rules')}</TabsTrigger>
            <TabsTrigger value='logs'>{t('Request Logs')}</TabsTrigger>
          </TabsList>
          <TabsContent value='rules'>
            <RequestRulesTable
              onCreateRule={handleCreate}
              onEditRule={handleEdit}
            />
          </TabsContent>
          <TabsContent value='logs'>
            <RequestLogsTable />
          </TabsContent>
        </Tabs>

        <RuleFormDialog
          open={formOpen}
          onOpenChange={setFormOpen}
          rule={editingRule}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
