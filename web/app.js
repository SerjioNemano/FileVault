let selectedDirId
let rootDirId
let dirsMap = {}
let currentFiles = []

// Выделенные файлы текущей папки; lastId — опора для Shift+клик
const selection = {
    ids: new Set(),
    lastId: null,
}

const viewableTypes = [
    'image/jpeg',
    'image/png',
    'image/gif',
    'image/webp',
    'video/mp4',
    'video/webm',
]

const isViewable = mimeType => viewableTypes.includes(mimeType)

const uploadTexts = {
    title: 'Загрузка файлов',
    running: 'Идет загрузка',
    done: 'Файлы успешно загружены',
    failed: 'Загрузка завершена с ошибками',
    stopped: 'Загрузка остановлена',
}

const deleteTexts = {
    title: 'Удаление файлов',
    running: 'Идет удаление',
    done: 'Файлы удалены',
    failed: 'Удаление завершено с ошибками',
    stopped: 'Удаление остановлено',
}

const moveTexts = {
    title: 'Перенос файлов',
    running: 'Идет перенос',
    done: 'Файлы перенесены',
    failed: 'Перенос завершен с ошибками',
    stopped: 'Перенос остановлен',
}

const progressModal = {
    modal: $('#progress-modal'),
    title: $('#progress-title'),
    status: $('#progress-status'),
    progressBar: $('#progress-bar'),
    progressText: $('#progress-text'),
    errors: $('#progress-errors'),
    closeButton: $('#progress-close'),
    stopButton: $('#progress-stop'),
    isOpen: false,
    stopped: false,
    texts: null,

    show(texts, total) {
        this.texts = texts;
        this.stopped = false;
        this.stopButton.text('Стоп').prop({ hidden: false, disabled: false });

        this.isOpen = true;
        this.modal.css('display','flex');

        this.title.text(texts.title);
        this.status.text(texts.running);

        this.progressBar.css('width', '0%');
        this.progressText.text(`0 / ${total}`);

        this.errors.empty()

        this.closeButton.prop('disabled', true);
    },

    updateProgress(processed, total) {
        const percent = total === 0
            ? 0
            : Math.round((processed / total) * 100);

        this.progressBar.css('width',`${percent}%`);
        this.progressText.text(`${processed} / ${total}`);
    },

    addError(filename, message) {
        const error = $('<div>').addClass('upload-error');
        const name = $('<strong>').text(`${filename}: `);

        error.append(name, document.createTextNode(String(message)));
        this.errors.append(error);

        this.errors.scrollTop(this.errors[0].scrollHeight);
    },

    // Текущий файл обрабатывается до конца, следующий не начинается
    stop() {
        this.stopped = true;
        this.stopButton.text('Останавливаю…').prop('disabled', true);
    },

    finish(success) {
        if (this.stopped) {
            this.status.text(this.texts.stopped);
        } else {
            this.status.text(success ? this.texts.done : this.texts.failed);
        }

        this.stopButton.prop('hidden', true);
        this.closeButton.prop('disabled', false);
    },

    hide() {
        this.isOpen = false;
        this.modal.css('display', 'none');
    }
};

const dirModal = {
    modal: $('#dir-modal'),
    name: $('#dir-name'),
    description: $('#dir-description'),
    error: $('#dir-error'),
    submitButton: $('#dir-submit'),

    show() {
        this.name.val('')
        this.description.val('')
        this.error.addClass('hide').text('')
        this.submitButton.prop('disabled', false)

        this.modal.css('display', 'flex')
        this.name.trigger('focus')
    },

    showError(message) {
        this.error.text(message).removeClass('hide')
    },

    hide() {
        this.modal.css('display', 'none')
    }
}

// Расширение — с последней точки, как filepath.Ext на сервере
const splitName = name => {
    const dot = name.lastIndexOf('.')
    return dot >= 0 ? [name.slice(0, dot), name.slice(dot)] : [name, '']
}

// Одно окно для файлов и папок: save(новое имя) возвращает промис,
// у папки расширения нет — ext пустой
const renameModal = {
    modal: $('#rename-modal'),
    name: $('#rename-name'),
    ext: $('#rename-ext'),
    error: $('#rename-error'),
    submitButton: $('#rename-submit'),
    save: null,

    show(name, ext, save) {
        this.save = save
        this.name.val(name)
        this.ext.text(ext)
        this.error.addClass('hide').text('')
        this.submitButton.prop('disabled', false)

        this.modal.css('display', 'flex')
        this.name.trigger('focus').trigger('select')
    },

    showError(message) {
        this.error.text(message).removeClass('hide')
    },

    hide() {
        this.modal.css('display', 'none')
        this.save = null
    }
}

const moveModal = {
    modal: $('#move-modal'),
    title: $('#move-title'),
    list: $('#move-list'),
    files: [],

    show(files) {
        const dirIds = [rootDirId, ...$('#dirs').children().map((i, row) => $(row).data('dir-id')).get()]

        this.files = files
        this.title.text(`Переместить ${pluralFiles(files.length)}`)
        this.list.empty()
        dirIds
            .filter(dirId => dirId !== selectedDirId && dirsMap[dirId])
            .forEach(dirId => {
                const dir = dirsMap[dirId]
                this.list.append($('<button>')
                    .attr('type', 'button')
                    .addClass('move-list__item')
                    .attr('data-move-dir-id', dir.id)
                    .text(`📁 ${dir.is_root ? 'Мои файлы' : dir.name}`))
            })

        this.modal.css('display', 'flex')
        this.list.find('button').first().trigger('focus')
    },

    hide() {
        this.modal.css('display', 'none')
        this.files = []
    }
}

const dirMenu = {
    menu: $('#dir-menu'),
    dirId: null,
    row: null,

    // Меню одно на все папки: ставится под нажатой кнопкой, position: fixed —
    // прокрутка сайдбара его не обрежет
    open(button, dirId) {
        const rect = button.getBoundingClientRect()

        this.dirId = dirId
        this.row = $(button).closest('.tree-item').addClass('tree-item--menu-open')
        this.menu.removeClass('hide').css({
            top: rect.bottom + 2,
            left: rect.right - this.menu.outerWidth(),
        })
    },

    close() {
        this.menu.addClass('hide')
        this.row?.removeClass('tree-item--menu-open')
        this.dirId = null
        this.row = null
    }
}

const getDirElemForSidebar = (dirId, name, fileCount) => {
    let dir = $('<div>').addClass('tree-item').attr('data-dir-id', dirId)
    let icon = $('<span>').addClass('tree-icon').text('📁')
    let dirName = $('<span>').addClass('tree-name').text(name)
    let fileCountElem = $('<span>').addClass('fileCount').text(` (${fileCount})`)
    let menuButton = $('<button>')
        .attr('type', 'button')
        .addClass('tree-menu')
        .attr('title', 'Действия с папкой')
        .text('⋯')

    dir.append(icon)
    dir.append(dirName)
    dir.append(fileCountElem)
    dir.append(menuButton)

    return dir
}

const getFileElem = file => {
    let card = $('<div>').addClass('file-card').attr('data-file-id', file.id)
    let previewContainer = $('<div>').addClass('file-preview')
    let previewIMG = $('<img>')
        .attr('src', file.has_preview ? `/api/files/${file.id}/preview` : '/img/preview.png')
        .attr('alt', file.original_name)
        .attr('loading', 'lazy')
        .one('error', function () {
            $(this).attr('src', '/img/preview.png')
        })
    let fileName = $('<div>').addClass('file-name').text(file.original_name)

    let selectButton = $('<button>')
        .attr('type', 'button')
        .addClass('file-select')
        .attr('title', 'Выделить')
        .text('✓')
    let actions = $('<div>').addClass('file-actions')
    let renameButton = $('<button>')
        .attr('type', 'button')
        .addClass('file-action file-rename')
        .attr('title', 'Переименовать')
        .text('✏️')
    let moveButton = $('<button>')
        .attr('type', 'button')
        .addClass('file-action file-move')
        .attr('title', 'Переместить')
        .text('📂')
    let downloadLink = $('<a>')
        .addClass('file-action file-download')
        .attr('href', `/api/files/${file.id}/original?download=1`)
        .attr('title', 'Скачать')
        .text('⬇')
    let deleteButton = $('<button>')
        .attr('type', 'button')
        .addClass('file-action file-delete')
        .attr('title', 'Удалить')
        .text('🗑')

    actions.append(renameButton)
    actions.append(moveButton)
    actions.append(downloadLink)
    actions.append(deleteButton)

    previewContainer.append(previewIMG)
    previewContainer.append(selectButton)
    previewContainer.append(actions)

    card.append(previewContainer)
    card.append(fileName)

    if (!isViewable(file.mime_type)) {
        card.addClass('file-card--download-only')
    }

    return card
}
const addRowInDirs = dir => {
    $('#dirs').append(getDirElemForSidebar(dir.id, dir.name, dir.file_count));
}

const insertRowInDirs = dir => {
    const elem = getDirElemForSidebar(dir.id, dir.name, dir.file_count)
    const next = $('#dirs').children().filter(function () {
        return dirsMap[$(this).data('dir-id')].name.localeCompare(dir.name, 'en-US') > 0
    }).first()

    if (next.length) {
        next.before(elem)
    } else {
        $('#dirs').append(elem)
    }
}

const addFileInDir = file => {
    $('#files').append(getFileElem(file))
}

const loadDirFiles = dirId => {
    fetch('/api/dirs/' + dirId + '/files')
    .then(response => {
        if (!response.ok) throw new Error('HTTP ' + response.status);
        return response.json();
    })
    .then(files => {
        if (dirId !== selectedDirId) return;
        currentFiles = files
        $('#files').empty()
        files.forEach(file => {
            addFileInDir(file);
        })
    })
    .catch(err => console.error(err));
}

const localTime = ms => {
    const date = new Date(ms)
    const pad = n => String(n).padStart(2, '0')
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
        `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

const uploadFile = (file, dirId) => {
    const form = new FormData()
    form.append('data', JSON.stringify({ modtime: localTime(file.lastModified), description: '' }))
    form.append('file', file)

    return fetch('/api/dirs/' + dirId + '/files', { method: 'POST', body: form })
        .then(response => {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(saved => {
            if (dirId !== selectedDirId) return;
            currentFiles.push(saved)
            addFileInDir(saved)
        })
}

const requestDeleteFile = fileId => {
    return fetch('/api/files/' + fileId, { method: 'DELETE' })
        .then(response => {
            if (!response.ok && response.status !== 404) throw new Error('HTTP ' + response.status);
        })
}

const removeFileCard = (fileId, dirId) => {
    if (dirId !== selectedDirId) return;
    currentFiles = currentFiles.filter(file => file.id !== fileId)
    $(`[data-file-id='${fileId}']`).remove()
}

const deleteFile = fileId => {
    const dirId = selectedDirId

    return requestDeleteFile(fileId)
        .then(() => {
            updateCountFilesInDir(dirId)
            removeFileCard(fileId, dirId)
        })
}

const moveFileErrors = {
    404: 'файл или папка уже удалены',
    409: 'в папке уже есть файл с таким именем',
}

const requestMoveFile = (fileId, dirId) => {
    return fetch(`/api/files/${fileId}/move`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dir_id: dirId }),
    })
        .then(response => {
            if (!response.ok) throw new Error(moveFileErrors[response.status] || 'не удалось перенести')
        })
}

const moveFiles = async (files, targetDirId) => {
    const sourceDirId = selectedDirId
    let processedCounter = 0
    let hasErrors = false

    moveModal.hide()
    progressModal.show(moveTexts, files.length)
    for (const file of files) {
        if (progressModal.stopped) break
        try {
            await requestMoveFile(file.id, targetDirId)
            selection.ids.delete(file.id)
            removeFileCard(file.id, sourceDirId)
        } catch (err) {
            console.error(file.original_name, err)
            progressModal.addError(file.original_name, err.message)
            hasErrors = true
        }
        processedCounter ++
        progressModal.updateProgress(processedCounter, files.length)
    }
    updateCountFilesInDir(sourceDirId)
    updateCountFilesInDir(targetDirId)
    renderSelection()
    progressModal.finish(!hasErrors)
}

const renderSelection = () => {
    const count = selection.ids.size

    $('#files [data-file-id]').each(function () {
        $(this).toggleClass('file-card--selected', selection.ids.has($(this).data('file-id')))
    })
    $('#files').toggleClass('file-grid--selecting', count > 0)
    $('.toolbar').toggleClass('toolbar--selecting', count > 0)
    $('#selection-count').text(`Выбрано: ${count}`)
}

const clearSelection = () => {
    selection.ids.clear()
    selection.lastId = null
    renderSelection()
}

const toggleSelect = (fileId, range) => {
    const ids = currentFiles.map(file => file.id)
    const from = ids.indexOf(selection.lastId)
    const to = ids.indexOf(fileId)

    if (range && from !== -1 && to !== -1) {
        ids.slice(Math.min(from, to), Math.max(from, to) + 1).forEach(id => selection.ids.add(id))
    } else if (selection.ids.has(fileId)) {
        selection.ids.delete(fileId)
    } else {
        selection.ids.add(fileId)
    }

    selection.lastId = fileId
    renderSelection()
}

const pluralFiles = count => {
    const lastTwo = count % 100
    const last = count % 10

    if (lastTwo >= 11 && lastTwo <= 14) return `${count} файлов`
    if (last === 1) return `${count} файл`
    if (last >= 2 && last <= 4) return `${count} файла`
    return `${count} файлов`
}

const createDirErrors = {
    400: 'Недопустимое имя: оно не может быть пустым, «.» или «..» и содержать / \\ < > : " | ? *',
    409: 'Папка с таким именем уже есть',
    413: 'Слишком длинное описание',
}

const createDir = (name, description) => {
    return fetch('/api/dirs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, description }),
    })
        .then(response => {
            if (!response.ok) {
                throw new Error(createDirErrors[response.status] || 'Не удалось создать папку')
            }
            return response.json()
        })
        .then(dir => {
            dirsMap[dir.id] = dir
            insertRowInDirs(dir)
            return dir
        })
}

const renameFileErrors = {
    400: 'Недопустимое имя: оно не может быть пустым и содержать / \\ < > : " | ? *',
    404: 'Файл уже удалён',
    409: 'Файл с таким именем уже есть в этой папке',
}

const renameFile = (file, base) => {
    return fetch('/api/files/' + file.id + '/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: base }),
    })
        .then(response => {
            if (!response.ok) {
                throw new Error(renameFileErrors[response.status] || 'Не удалось переименовать файл')
            }
            return response.json()
        })
        .then(saved => {
            // Тот же объект лежит и в currentFiles, и в viewer.files —
            // меняем на месте, чтобы обновились оба списка
            Object.assign(file, saved)

            $(`[data-file-id='${file.id}']`)
                .find('.file-name').text(file.original_name).end()
                .find('.file-preview img').attr('alt', file.original_name)

            return file
        })
}

const renameDirErrors = {
    400: 'Недопустимое имя: оно не может быть пустым, «.» или «..» и содержать / \\ < > : " | ? *',
    404: 'Папка уже удалена',
    409: 'Папка с таким именем уже есть',
}

const renameDir = (dir, name) => {
    return fetch(`/api/dirs/${dir.id}/rename`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
    })
        .then(response => {
            if (!response.ok) {
                throw new Error(renameDirErrors[response.status] || 'Не удалось переименовать папку')
            }
            return response.json()
        })
        .then(saved => {
            Object.assign(dir, saved)

            // Новое имя — новое место по алфавиту: строку переставляем
            $('#dirs').children(`[data-dir-id='${dir.id}']`).remove()
            insertRowInDirs(dir)

            if (dir.id === selectedDirId) {
                $(`[data-dir-id='${dir.id}']`).addClass('tree-item--active')
                breadCrumb(dir)
            }
        })
}

const dirNotEmptyMessage = dir =>
    `В папке «${dir.name}» есть файлы. Сначала удалите или перенесите их.`

// Удаляется только пустая папка: файлы годами копятся, а бэкапа пока нет.
// 404 — тоже удаление: папки уже нет
const deleteDir = dir => {
    return fetch(`/api/dirs/${dir.id}`, { method: 'DELETE' })
        .then(response => {
            if (response.status === 409) throw new Error(dirNotEmptyMessage(dir))
            if (!response.ok && response.status !== 404) throw new Error('Не удалось удалить папку')
        })
        .then(() => {
            $('#dirs').children(`[data-dir-id='${dir.id}']`).remove()
            delete dirsMap[dir.id]

            if (dir.id === selectedDirId) selectDir(rootDirId)
        })
}

const openFileRename = file => {
    const [base, ext] = splitName(file.original_name)

    renameModal.show(base, ext, newBase => renameFile(file, newBase).then(renamed => {
        if (viewer.isOpen && viewer.files[viewer.index] === renamed) {
            viewer.name.text(renamed.original_name)
        }
    }))
}

const openDirRename = dir => renameModal.show(dir.name, '', newName => renameDir(dir, newName))

const confirmDelete = file => confirm(`Удалить «${file.original_name}»? Отменить будет нельзя.`)

const breadCrumb = (dir) => {
    if (!dir.is_root) {
        $('#breadcrumb-name').text(dir.name)
        $('#breadcrumb-separator').removeClass('hide')
    } else {
        $('#breadcrumb-name').text('')
        $('#breadcrumb-separator').addClass('hide')
    }
}

const loadDirs = () => {
    fetch('/api/dirs')
        .then(response => {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(dirs => {
            dirs.forEach(dir => {
                dirsMap[dir.id] = dir

                if (dir.is_root) {
                    rootDirId = dir.id;
                    $('#root-dir').attr('data-dir-id', rootDirId)
                    $('#to-root').prop('disabled', false);
                } else {
                    addRowInDirs(dir);
                }
            })
            selectDir(rootDirId);
        })
        .catch(err => console.error(err));
}

const updateCountFilesInDir = dirId => {
    const url = `/api/dirs/${dirId}/count_files`
    fetch(url)
        .then(response => {
            if (!response.ok) throw new Error('HTTP ' + response.status);
            return response.json();
        })
        .then(fileCount => {
            const fileCountElem = $(`[data-dir-id='${dirId}']`).find('.fileCount')
            fileCountElem.text(` (${fileCount['file_count']})`)
            dirsMap[dirId].file_count = fileCount.file_count
        })
        .catch(err => console.error(err));
}

const selectDir = dirId => {
    if (dirId === selectedDirId) return;

    selectedDirId = dirId;
    clearSelection()

    $('[data-dir-id]').removeClass('tree-item--active');

    $(`[data-dir-id='${dirId}']`).addClass('tree-item--active')

    breadCrumb(dirsMap[dirId])

    loadDirFiles(dirId)
}

loadDirs()

$(document).on('click', '[data-dir-id]', function () {
    selectDir($(this).data('dir-id'))
})

$('#to-root').on('click', () => selectDir(rootDirId))

$(document).on('click', '[data-file-id]', function (event) {
    const fileId = $(this).data('file-id')

    if (selection.ids.size > 0) {
        toggleSelect(fileId, event.shiftKey)
        return
    }

    const file = currentFiles.find(f => f.id === fileId)
    if (!file || !isViewable(file.mime_type)) return

    viewer.open(fileId)
})

// Иконки лежат внутри карточки: без этого клик «скачать» у фото
// заодно открыл бы просмотрщик
$(document).on('click', '.file-download', event => event.stopPropagation())

$(document).on('click', '.file-select', function (event) {
    event.stopPropagation()
    toggleSelect($(this).closest('[data-file-id]').data('file-id'), event.shiftKey)
})

$('#selection-clear').on('click', () => clearSelection())

$(document).on('click', '.file-move', function (event) {
    event.stopPropagation()

    const fileId = $(this).closest('[data-file-id]').data('file-id')
    const file = currentFiles.find(f => f.id === fileId)
    if (file) moveModal.show([file])
})

$('#selection-move').on('click', () => {
    const files = currentFiles.filter(file => selection.ids.has(file.id))
    if (files.length > 0) moveModal.show(files)
})

$('#move-list').on('click', '[data-move-dir-id]', function () {
    moveFiles(moveModal.files, $(this).data('move-dir-id'))
})

$('#move-cancel').on('click', () => moveModal.hide())

$('#move-modal').on('keydown', event => {
    if (event.key !== 'Escape') return

    event.stopPropagation()
    moveModal.hide()
})

$(document).on('keydown', event => {
    if (event.key === 'Escape' && !viewer.isOpen && !progressModal.isOpen && selection.ids.size > 0) clearSelection()
})

$('#selection-delete').on('click', async function () {
    const files = currentFiles.filter(file => selection.ids.has(file.id))
    if (files.length === 0) return
    if (!confirm(`Удалить ${pluralFiles(files.length)}? Отменить будет нельзя.`)) return

    const dirId = selectedDirId
    let processedCounter = 0
    let hasErrors = false

    progressModal.show(deleteTexts, files.length)
    for (const file of files) {
        if (progressModal.stopped) break
        try {
            await requestDeleteFile(file.id)
            selection.ids.delete(file.id)
            removeFileCard(file.id, dirId)
        } catch (err) {
            console.error(file.original_name, err)
            progressModal.addError(file.original_name, err.message)
            hasErrors = true
        }
        processedCounter ++
        progressModal.updateProgress(processedCounter, files.length)
    }
    updateCountFilesInDir(dirId)
    renderSelection()
    progressModal.finish(!hasErrors)
})

$(document).on('click', '.file-delete', function (event) {
    event.stopPropagation()

    const button = $(this)
    const fileId = button.closest('[data-file-id]').data('file-id')
    const file = currentFiles.find(f => f.id === fileId)
    if (!file || !confirmDelete(file)) return

    button.prop('disabled', true)
    deleteFile(fileId).catch(err => {
        console.error(err)
        button.prop('disabled', false)
        alert('Не удалось удалить файл')
    })
})

$(document).on('click', '.file-rename', function (event) {
    event.stopPropagation()

    const fileId = $(this).closest('[data-file-id]').data('file-id')
    const file = currentFiles.find(f => f.id === fileId)
    if (file) openFileRename(file)
})

$(document).on('click', '.tree-menu', function (event) {
    event.stopPropagation()

    const dirId = $(this).closest('[data-dir-id]').data('dir-id')
    if (dirMenu.dirId === dirId) {
        dirMenu.close()
        return
    }
    dirMenu.open(this, dirId)
})

$('#dir-menu-rename').on('click', () => {
    const dir = dirsMap[dirMenu.dirId]
    dirMenu.close()
    if (dir) openDirRename(dir)
})

// Счётчик на клиенте — подсказка, чтобы не спрашивать «удалить?» зря;
// решает всё равно сервер (409)
$('#dir-menu-delete').on('click', () => {
    const dir = dirsMap[dirMenu.dirId]
    dirMenu.close()
    if (!dir) return

    if (dir.file_count > 0) {
        alert(dirNotEmptyMessage(dir))
        return
    }
    if (!confirm(`Удалить папку «${dir.name}»?`)) return

    deleteDir(dir).catch(err => {
        console.error(err)
        alert(err.message)
    })
})

$(document).on('click', event => {
    if (!$(event.target).closest('#dir-menu').length) dirMenu.close()
})

$(document).on('keydown', event => {
    if (event.key === 'Escape') dirMenu.close()
})

$('.sidebar').on('scroll', () => dirMenu.close())
$(window).on('resize', () => dirMenu.close())

$('#rename-cancel').on('click', () => renameModal.hide())

$('#rename-form').on('keydown', event => {
    if (event.key !== 'Escape') return

    event.stopPropagation()
    renameModal.hide()
})

$('#rename-form').on('submit', function (event) {
    event.preventDefault()

    renameModal.submitButton.prop('disabled', true)
    renameModal.save(renameModal.name.val())
        .then(() => renameModal.hide())
        .catch(err => {
            console.error(err)
            renameModal.showError(err.message)
            renameModal.submitButton.prop('disabled', false)
        })
})

$('#add-files').on('click', () => $('#file-input').trigger('click'))

$('#file-input').on('change', async function () {
    const dirId = selectedDirId
    const files = [...this.files]
    const totalFiles = files.length

    let processedCounter = 0
    let hasErrors = false

    this.value = ''

    progressModal.show(uploadTexts, totalFiles)
    for (const file of files) {
        if (progressModal.stopped) break
        try {
            await uploadFile(file, dirId)
        } catch (err) {
            console.error(file.name, err)
            progressModal.addError(file.name, err.message)
            hasErrors = true
        }
        processedCounter ++
        progressModal.updateProgress(processedCounter, totalFiles)
    }
    updateCountFilesInDir(dirId)
    progressModal.finish(!hasErrors)
})

$('#create-dir').on('click', () => dirModal.show())

$('#dir-cancel').on('click', () => dirModal.hide())

$('#dir-form').on('keydown', event => {
    if (event.key === 'Escape') dirModal.hide()
})

$('#dir-form').on('submit', function (event) {
    event.preventDefault()

    dirModal.submitButton.prop('disabled', true)
    createDir(dirModal.name.val(), dirModal.description.val())
        .then(dir => {
            dirModal.hide()
            selectDir(dir.id)
        })
        .catch(err => {
            console.error(err)
            dirModal.showError(err.message)
            dirModal.submitButton.prop('disabled', false)
        })
})

progressModal.closeButton.on('click', () => {
    progressModal.hide();
});

progressModal.stopButton.on('click', () => progressModal.stop());