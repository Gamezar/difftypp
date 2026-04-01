/**
 * Index page entry point.
 *
 * Initializes the file explorer modal for browsing Git repositories.
 */

import { initializeFileExplorer } from './file-explorer'
function showRemoveModal(repoPath: string): Promise<boolean> {
  const overlay = document.getElementById('remove-modal') as HTMLDivElement | null
  const pathEl = document.getElementById('remove-modal-path') as HTMLParagraphElement | null
  const confirmBtn = document.getElementById('remove-modal-confirm') as HTMLButtonElement | null
  const cancelBtn = document.getElementById('remove-modal-cancel') as HTMLButtonElement | null
  const closeBtn = document.getElementById('remove-modal-close') as HTMLButtonElement | null

  if (!overlay || !pathEl || !confirmBtn || !cancelBtn || !closeBtn) return Promise.resolve(false)

  // Re-bind after null guard so TypeScript narrows inside closures
  const modal = overlay
  const confirm = confirmBtn
  const cancel = cancelBtn
  const close = closeBtn

  pathEl.textContent = repoPath
  modal.style.display = ''

  return new Promise(resolve => {
    function cleanup(result: boolean) {
      modal.style.display = 'none'
      confirm.removeEventListener('click', onConfirm)
      cancel.removeEventListener('click', onCancel)
      close.removeEventListener('click', onCancel)
      modal.removeEventListener('click', onOverlay)
      resolve(result)
    }

    function onConfirm() { cleanup(true) }
    function onCancel() { cleanup(false) }
    function onOverlay(e: MouseEvent) {
      if (e.target === modal) cleanup(false)
    }

    confirm.addEventListener('click', onConfirm)
    cancel.addEventListener('click', onCancel)
    close.addEventListener('click', onCancel)
    modal.addEventListener('click', onOverlay)
  })
}

function initializeRemoveButtons() {
  document.querySelectorAll<HTMLButtonElement>('.btn-remove-repo').forEach(btn => {
    btn.addEventListener('click', async () => {
      const repoPath = btn.dataset.repoPath
      if (!repoPath) return

      const confirmed = await showRemoveModal(repoPath)
      if (!confirmed) return

      const resp = await fetch(`/api/repository/remove?path=${encodeURIComponent(repoPath)}`, {
        method: 'DELETE',
      })
      if (resp.ok) {
        window.location.reload()
      }
    })
  })
}

document.addEventListener('DOMContentLoaded', () => {
  initializeFileExplorer()
  initializeRemoveButtons()
})
