// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.desk.ui

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.ArrowCircleDown
import androidx.compose.material.icons.outlined.ArrowCircleUp
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Image
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import android.text.format.Formatter
import org.golink.desk.FilesModel
import org.golink.desk.R
import org.golink.desk.core.FileEntry
import org.golink.desk.core.FilesResult

/**
 * The computer's files, as on the Mac app: its home folder to browse, a
 * tap on a file downloads it (into Downloads › go-link Desk), and Send
 * takes a file from the Files app or a photo to the computer's
 * Downloads/go-link Desk.
 */
@Composable
fun FilesSheet(model: FilesModel, onDismiss: () -> Unit) {
    val path = remember { mutableStateListOf<String>() }
    val folder = path.lastOrNull() ?: ""
    var menu by remember { mutableStateOf(false) }
    val pickFile = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri -> uri?.let { model.upload(it) } }
    val pickPhoto = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri -> uri?.let { model.upload(it) } }
    val back = { if (path.isNotEmpty()) path.removeAt(path.size - 1) else onDismiss() }

    Sheet(
        if (folder.isEmpty()) stringResource(R.string.computers_files) else folder.substringAfterLast('/'),
        onDismiss,
        leading = {
            if (path.isEmpty()) {
                SheetAction(stringResource(R.string.done), bold = true, onClick = onDismiss)
            } else {
                IconButton(onClick = { back() }) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back), tint = Theme.accent) }
            }
        },
        trailing = {
            Box {
                TextButton(onClick = { menu = true }, enabled = model.progress == null) {
                    Icon(Icons.Outlined.IosShare, null, tint = Theme.accent, modifier = Modifier.size(18.dp))
                    Text(stringResource(R.string.send), color = Theme.accent, fontSize = 17.sp, modifier = Modifier.padding(start = 4.dp))
                }
                DropdownMenu(menu, { menu = false }) {
                    DropdownMenuItem(text = { Text(stringResource(R.string.from_files)) }, leadingIcon = { Icon(Icons.Outlined.Folder, null) }, onClick = {
                        menu = false
                        pickFile.launch(arrayOf("*/*"))
                    })
                    DropdownMenuItem(text = { Text(stringResource(R.string.from_photos)) }, leadingIcon = { Icon(Icons.Outlined.Image, null) }, onClick = {
                        menu = false
                        pickPhoto.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageAndVideo))
                    })
                }
            }
        },
    ) {
        androidx.activity.compose.BackHandler(path.isNotEmpty()) { back() }
        Box(Modifier.weight(1f)) {
            FolderView(model, folder) { path.add(if (folder.isEmpty()) it else "$folder/$it") }
        }
        TransferBar(model)
    }
}

/** One folder of the computer. */
@Composable
private fun FolderView(model: FilesModel, folder: String, open: (String) -> Unit) {
    var entries by remember(folder) { mutableStateOf<List<FileEntry>?>(null) }
    var error by remember(folder) { mutableStateOf<String?>(null) }
    var tries by remember(folder) { mutableStateOf(0) }
    LaunchedEffect(folder, tries) {
        error = null
        entries = null
        model.list(folder) { r ->
            when (r) {
                is FilesResult.Ok -> entries = r.value
                is FilesResult.Failed -> error = r.error.message
            }
        }
    }
    val list = entries
    when {
        list != null && list.isEmpty() -> Empty(Icons.Outlined.Folder, stringResource(R.string.folder_empty))
        list != null -> LazyColumn(Modifier.fillMaxSize()) {
            items(list, key = { it.name }) { e ->
                EntryRow(e, enabled = e.dir || model.progress == null) {
                    if (e.dir) open(e.name) else model.download(if (folder.isEmpty()) e.name else "$folder/${e.name}")
                }
                RowDivider(54)
            }
        }
        error != null -> Empty(Icons.Outlined.Warning, stringResource(R.string.cannot_open_folder), error) { tries++ }
        else -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator(color = Theme.accent) }
    }
}

@Composable
private fun Empty(icon: androidx.compose.ui.graphics.vector.ImageVector, title: String, detail: String? = null, retry: (() -> Unit)? = null) {
    Column(Modifier.fillMaxSize().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.Center) {
        Icon(icon, null, tint = colors.secondary, modifier = Modifier.size(44.dp))
        Text(title, fontSize = 20.sp, fontWeight = FontWeight.SemiBold, color = colors.text, textAlign = TextAlign.Center, modifier = Modifier.padding(top = 12.dp))
        if (detail != null) Text(detail, fontSize = 15.sp, color = colors.secondary, textAlign = TextAlign.Center, modifier = Modifier.padding(top = 6.dp))
        if (retry != null) TextButton(onClick = retry) { Text(stringResource(R.string.try_again), color = Theme.accent) }
    }
}

@Composable
private fun EntryRow(e: FileEntry, enabled: Boolean, onClick: () -> Unit) {
    val ctx = LocalContext.current
    val date = dateText(e.mtime * 1000, ctx)
    Row(Modifier.fillMaxWidth().clickable(enabled = enabled, onClick = onClick).padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(if (e.dir) Icons.Filled.Folder else Icons.Outlined.Description, null, tint = if (e.dir) Theme.accent else colors.secondary, modifier = Modifier.width(26.dp))
        Column(Modifier.padding(start = 12.dp).weight(1f)) {
            Text(e.name, fontSize = 17.sp, color = colors.text, maxLines = 1)
            Text(if (e.dir) date else Formatter.formatShortFileSize(ctx, e.size) + " · " + date, fontSize = 12.sp, color = colors.secondary)
        }
        if (!e.dir) Icon(Icons.Outlined.ArrowCircleDown, null, tint = Theme.accent)
    }
}

/** The transfer running, or what came of the last one. */
@Composable
private fun TransferBar(model: FilesModel) {
    val ctx = LocalContext.current
    val p = model.progress
    val o = model.outcome
    if (p == null && o == null) return
    Row(Modifier.fillMaxWidth().background(colors.card).navigationBarsPadding().padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
        if (p != null) {
            Icon(if (p.upload) Icons.Outlined.ArrowCircleUp else Icons.Outlined.ArrowCircleDown, null, tint = Theme.accent)
            Column(Modifier.padding(horizontal = 12.dp).weight(1f)) {
                Text(p.name, fontSize = 13.sp, color = colors.text, maxLines = 1)
                LinearProgressIndicator(progress = { p.fraction }, color = Theme.accent, modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp))
                Text("${Formatter.formatShortFileSize(ctx, p.done)} / ${Formatter.formatShortFileSize(ctx, p.size)}", fontSize = 11.sp, color = colors.secondary)
            }
            TextButton(onClick = { model.cancel() }) { Text(stringResource(R.string.cancel), color = Theme.accent, fontSize = 13.sp) }
        } else if (o != null) {
            when (o) {
                is FilesModel.Outcome.Downloaded -> {
                    Icon(Icons.Outlined.CheckCircle, null, tint = Color(0xFF34C759))
                    Text(stringResource(R.string.saved_in_downloads) + " " + o.name, fontSize = 13.sp, color = colors.text, maxLines = 2, modifier = Modifier.padding(horizontal = 12.dp).weight(1f))
                    IconButton(onClick = {
                        val send = Intent(Intent.ACTION_SEND).setType(o.mime).putExtra(Intent.EXTRA_STREAM, o.uri).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                        try {
                            ctx.startActivity(Intent.createChooser(send, o.name))
                        } catch (_: ActivityNotFoundException) {
                        }
                    }) { Icon(Icons.Outlined.IosShare, stringResource(R.string.share), tint = Theme.accent) }
                }
                is FilesModel.Outcome.Uploaded -> {
                    Icon(Icons.Outlined.CheckCircle, null, tint = Color(0xFF34C759))
                    Text(stringResource(R.string.sent_to_downloads) + " " + o.name, fontSize = 13.sp, color = colors.text, maxLines = 2, modifier = Modifier.padding(horizontal = 12.dp).weight(1f))
                }
                is FilesModel.Outcome.Failed -> {
                    Icon(Icons.Outlined.Warning, null, tint = Color(0xFFFF9F0A))
                    Text(o.why, fontSize = 13.sp, color = colors.text, maxLines = 2, modifier = Modifier.padding(horizontal = 12.dp).weight(1f))
                }
            }
            IconButton(onClick = { model.outcome = null }) { Icon(Icons.Outlined.Close, null, tint = colors.secondary) }
        }
    }
}
