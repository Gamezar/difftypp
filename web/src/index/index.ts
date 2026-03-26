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

  pathEl.textContent = repoPath
  overlay.style.display = ''

  return new Promise(resolve => {
    function cleanup(result: boolean) {
      overlay.style.display = 'none'
      confirmBtn.removeEventListener('click', onConfirm)
      cancelBtn.removeEventListener('click', onCancel)
      closeBtn.removeEventListener('click', onCancel)
      overlay.removeEventListener('click', onOverlay)
      resolve(result)
    }

    function onConfirm() { cleanup(true) }
    function onCancel() { cleanup(false) }
    function onOverlay(e: MouseEvent) {
      if (e.target === overlay) cleanup(false)
    }

    confirmBtn.addEventListener('click', onConfirm)
    cancelBtn.addEventListener('click', onCancel)
    closeBtn.addEventListener('click', onCancel)
    overlay.addEventListener('click', onOverlay)
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
