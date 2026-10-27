// Просмотрщик фото и видео на весь экран.
// Берёт файлы из currentFiles и листает только те, что проходят isViewable.
const viewer = {
    overlay: $('#viewer'),
    image: $('#viewer-image'),
    video: $('#viewer-video'),
    name: $('#viewer-name'),
    downloadLink: $('#viewer-download'),
    deleteButton: $('#viewer-delete'),
    renameButton: $('#viewer-rename'),
    prevButton: $('#viewer-prev'),
    nextButton: $('#viewer-next'),
    closeButton: $('#viewer-close'),
    error: $('#viewer-error'),
    placeholder: $('#viewer-placeholder'),
    spinner: $('#viewer-spinner'),

    files: [],
    index: 0,
    isOpen: false,

    open(fileId) {
        this.files = currentFiles.filter(file => isViewable(file.mime_type))
        this.index = this.files.findIndex(file => file.id === fileId)

        if (this.index === -1) return

        this.show()
        this.overlay.css('display', 'flex')
        this.isOpen = true
    },

    show() {
        const file = this.files[this.index]
        const url = `/api/files/${file.id}/original`

        this.stopVideo()
        this.error.addClass('hide')
        this.image.addClass('hide')
        this.video.addClass('hide')
        this.showLoading(file)

        if (file.mime_type.startsWith('video/')) {
            this.image.removeAttr('src')
            this.video.attr('src', url)
        } else {
            this.image.attr('src', url).attr('alt', file.original_name)
        }

        this.name.text(file.original_name)
        this.downloadLink.attr('href', `${url}?download=1`)

        this.prevButton.prop('disabled', this.index === 0)
        this.nextButton.prop('disabled', this.index === this.files.length - 1)
    },

    showLoading(file) {
        if (file.has_preview && !file.mime_type.startsWith('video/')) {
            this.placeholder.attr('src', `/api/files/${file.id}/preview`).removeClass('hide')
            this.spinner.addClass('hide')
        } else {
            this.placeholder.addClass('hide').removeAttr('src')
            this.spinner.removeClass('hide')
        }
    },

    hideLoading() {
        this.placeholder.addClass('hide').removeAttr('src')
        this.spinner.addClass('hide')
    },

    showMedia(media) {
        this.hideLoading()
        media.removeClass('hide')
    },

    next() {
        if (this.index >= this.files.length - 1) return

        this.index++
        this.show()
    },

    prev() {
        if (this.index <= 0) return

        this.index--
        this.show()
    },

    deleteCurrent() {
        const file = this.files[this.index]
        if (!confirmDelete(file)) return

        this.deleteButton.prop('disabled', true)
        deleteFile(file.id)
            .then(() => {
                this.files.splice(this.index, 1)
                if (this.files.length === 0) {
                    this.close()
                    return
                }
                this.index = Math.min(this.index, this.files.length - 1)
                this.show()
            })
            .catch(err => {
                console.error(err)
                alert('Не удалось удалить файл')
            })
            .finally(() => this.deleteButton.prop('disabled', false))
    },

    stopVideo() {
        const video = this.video[0]

        video.pause()
        video.removeAttribute('src')
        video.load()
    },

    showError() {
        this.hideLoading()
        this.image.addClass('hide')
        this.video.addClass('hide')
        this.error.removeClass('hide')
    },

    close() {
        this.stopVideo()
        this.hideLoading()
        this.image.removeAttr('src')
        this.overlay.css('display', 'none')
        this.isOpen = false
    },
}

viewer.prevButton.on('click', () => viewer.prev())
viewer.nextButton.on('click', () => viewer.next())
viewer.closeButton.on('click', () => viewer.close())
viewer.deleteButton.on('click', () => viewer.deleteCurrent())
viewer.renameButton.on('click', () => openFileRename(viewer.files[viewer.index]))

viewer.overlay.on('click', function (event) {
    if ($(event.target).is('#viewer, .viewer__stage, .viewer__header')) {
        viewer.close()
    }
})

viewer.image.on('load', function () {
    if ($(this).attr('src')) viewer.showMedia(viewer.image)
})

viewer.video.on('loadedmetadata', function () {
    if ($(this).attr('src')) viewer.showMedia(viewer.video)
})

viewer.placeholder.on('error', function () {
    if (!$(this).attr('src')) return

    $(this).addClass('hide').removeAttr('src')
    viewer.spinner.removeClass('hide')
})

viewer.image.on('error', function () {
    if ($(this).attr('src')) viewer.showError()
})

viewer.video.on('error', function () {
    if ($(this).attr('src')) viewer.showError()
})

$(document).on('keydown', function (event) {
    if (!viewer.isOpen) return

    if (event.key === 'Escape') {
        viewer.close()
        return
    }

    if (event.target === viewer.video[0] || $(event.target).is('input, textarea')) return

    if (event.key === 'ArrowLeft') viewer.prev()
    if (event.key === 'ArrowRight') viewer.next()
})
