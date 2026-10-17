import React, { useState, useEffect } from "react";
import { Button } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { faTrashAlt } from "@fortawesome/free-solid-svg-icons";
import Mousetrap from "mousetrap";
import * as GQL from "src/core/generated-graphql";
import { ChapterEntries } from "./ChapterEntry";
import { GalleryChapterForm } from "./GalleryChapterForm";
import { usePermissions } from "src/hooks/Permissions";
import { useGalleryChapterDestroy } from "src/core/StashService";
import { useToast } from "src/hooks/Toast";
import { ModalComponent } from "src/components/Shared/Modal";

interface IGalleryChapterPanelProps {
  gallery: GQL.GalleryDataFragment;
  isVisible: boolean;
  onClickChapter: (index: number) => void;
}

export const GalleryChapterPanel: React.FC<IGalleryChapterPanelProps> = ({
  gallery,
  isVisible,
  onClickChapter,
}) => {
  const { can } = usePermissions();
  const canEdit = can(GQL.Permission.ChaptersEdit);
  const canDelete = can(GQL.Permission.ChaptersDelete);
  const intl = useIntl();
  const Toast = useToast();
  const [galleryChapterDestroy] = useGalleryChapterDestroy();
  const [isEditorOpen, setIsEditorOpen] = useState<boolean>(false);
  const [editingChapter, setEditingChapter] =
    useState<GQL.GalleryChapterDataFragment>();
  const [deletingChapter, setDeletingChapter] =
    useState<GQL.GalleryChapterDataFragment>();

  // set up hotkeys
  useEffect(() => {
    if (!isVisible || !canEdit) return;

    Mousetrap.bind("n", () => onOpenEditor());

    return () => {
      Mousetrap.unbind("n");
    };
  });

  function onOpenEditor(chapter?: GQL.GalleryChapterDataFragment) {
    setIsEditorOpen(true);
    setEditingChapter(chapter ?? undefined);
  }

  const closeEditor = () => {
    setEditingChapter(undefined);
    setIsEditorOpen(false);
  };

  async function onDeleteChapter(chapter: GQL.GalleryChapterDataFragment) {
    setDeletingChapter(undefined);
    try {
      await galleryChapterDestroy({ variables: { id: chapter.id } });
    } catch (e) {
      Toast.error(e);
    }
  }

  if (isEditorOpen)
    return (
      <GalleryChapterForm
        galleryID={gallery.id}
        chapter={editingChapter}
        onClose={closeEditor}
      />
    );

  return (
    <div>
      <ModalComponent
        show={!!deletingChapter}
        icon={faTrashAlt}
        header={intl.formatMessage({ id: "actions.delete" })}
        accept={{
          text: intl.formatMessage({ id: "actions.delete" }),
          variant: "danger",
          onClick: () => deletingChapter && onDeleteChapter(deletingChapter),
        }}
        cancel={{
          onClick: () => setDeletingChapter(undefined),
          variant: "secondary",
        }}
      >
        <p>
          {intl.formatMessage(
            { id: "dialogs.delete_confirm" },
            {
              entityName:
                deletingChapter?.title || `#${deletingChapter?.image_index}`,
            }
          )}
        </p>
      </ModalComponent>
      {canEdit && (
        <Button onClick={() => onOpenEditor()}>
          <FormattedMessage id="actions.create_chapters" />
        </Button>
      )}
      <div className="container">
        <ChapterEntries
          galleryChapters={gallery.chapters}
          onClickChapter={onClickChapter}
          onEdit={canEdit ? onOpenEditor : undefined}
          onDelete={!canEdit && canDelete ? setDeletingChapter : undefined}
        />
      </div>
    </div>
  );
};

export default GalleryChapterPanel;
