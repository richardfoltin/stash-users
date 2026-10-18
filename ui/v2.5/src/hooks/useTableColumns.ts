import { useConfigureUISetting } from "src/core/StashService";
import { useConfigurationContext } from "src/hooks/Config";
import { useToast } from "./Toast";

export const useTableColumns = (
  tableName: string,
  defaultColumns: string[]
) => {
  const Toast = useToast();

  const { configuration } = useConfigurationContext();
  const [saveUISetting] = useConfigureUISetting();

  const ui = configuration?.ui;

  const selectedColumns = ui?.tableColumns?.[tableName] ?? defaultColumns;

  async function saveColumns(updatedColumns: string[]) {
    try {
      await saveUISetting({
        variables: {
          key: `tableColumns.${tableName}`,
          value: updatedColumns,
        },
      });
    } catch (e) {
      Toast.error(e);
    }
  }

  return { selectedColumns, saveColumns };
};
