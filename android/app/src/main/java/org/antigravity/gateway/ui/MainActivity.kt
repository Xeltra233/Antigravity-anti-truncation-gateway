package org.antigravity.gateway.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.net.Uri
import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.text.method.ScrollingMovementMethod
import android.view.LayoutInflater
import android.view.View
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.RadioButton
import android.widget.TextView
import android.widget.Toast
import androidx.activity.viewModels
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import kotlinx.coroutines.launch
import org.antigravity.gateway.R
import org.antigravity.gateway.databinding.ActivityMainBinding

class MainActivity : AppCompatActivity() {

    private lateinit var binding: ActivityMainBinding
    private val viewModel: MainViewModel by viewModels()

    private var isInternalTextUpdating = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.tvTestResult.movementMethod = ScrollingMovementMethod()

        setupListeners()
        observeUiState()
    }

    override fun onResume() {
        super.onResume()
        viewModel.loadConfigAndNetwork()
    }

    private fun setupListeners() {
        // Provider switcher & rename
        binding.layoutProviderSelect.setOnClickListener { showProviderDialog() }
        binding.btnSwitchProvider.setOnClickListener { showProviderDialog() }
        binding.btnRenameCurrentProvider.setOnClickListener {
            val current = viewModel.uiState.value.config.getCurrentProvider()
            if (current != null) {
                showRenameProviderDialog(current.id, current.name)
            }
        }
        binding.tvCurrentProvider.setOnLongClickListener {
            val current = viewModel.uiState.value.config.getCurrentProvider()
            if (current != null) {
                showRenameProviderDialog(current.id, current.name)
            }
            true
        }

        // Upstream URL & Key text watchers
        val textWatcher = object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) {
                if (isInternalTextUpdating) return
                val url = binding.etUpstreamUrl.text?.toString() ?: ""
                val key = binding.etUpstreamKey.text?.toString() ?: ""
                viewModel.updateDraft(url, key)
            }
        }
        binding.etUpstreamUrl.addTextChangedListener(textWatcher)
        binding.etUpstreamKey.addTextChangedListener(textWatcher)

        // Downstream Key Generate & Copy
        binding.btnGenerateKey.setOnClickListener {
            viewModel.regenerateDownstreamKey()
        }
        binding.btnCopyKey.setOnClickListener {
            val key = viewModel.uiState.value.config.downstreamKey
            if (key.isNotEmpty()) {
                copyToClipboard("Downstream Key", key)
                Toast.makeText(this, "已复制下游 Key", Toast.LENGTH_SHORT).show()
            }
        }

        // App Link Copy
        binding.btnCopyLink.setOnClickListener {
            val link = viewModel.uiState.value.appLink
            copyToClipboard("App Link", link)
            Toast.makeText(this, "已复制应用链接", Toast.LENGTH_SHORT).show()
        }

        // Start / Stop Gateway
        binding.btnStartStop.setOnClickListener {
            val currentState = viewModel.uiState.value.gatewayState
            if (currentState == GatewayState.RUNNING) {
                viewModel.stopGateway(this)
            } else {
                viewModel.startGateway(this)
            }
        }

        // Fetch Models
        binding.btnFetchModels.setOnClickListener {
            viewModel.fetchModels()
        }

        // Test Message
        binding.btnTestMessage.setOnClickListener {
            val customModel = binding.etTestModelId.text?.toString() ?: ""
            viewModel.testMessage(customModel)
        }
    }

    private fun observeUiState() {
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                viewModel.uiState.collect { state ->
                    renderUi(state)
                }
            }
        }
    }

    private fun renderUi(state: UiState) {
        val currentProvider = state.config.getCurrentProvider()

        // 1. Provider header
        binding.tvCurrentProvider.text = currentProvider?.name ?: "未选择供应商"

        // 2. Input fields (update only if changed to avoid cursor jumping)
        isInternalTextUpdating = true
        val targetUrl = currentProvider?.upstreamUrl ?: ""
        if (binding.etUpstreamUrl.text?.toString() != targetUrl) {
            binding.etUpstreamUrl.setText(targetUrl)
        }
        val targetKey = currentProvider?.upstreamKey ?: ""
        if (binding.etUpstreamKey.text?.toString() != targetKey) {
            binding.etUpstreamKey.setText(targetKey)
        }
        val targetDownstream = state.config.downstreamKey
        if (binding.etDownstreamKey.text?.toString() != targetDownstream) {
            binding.etDownstreamKey.setText(targetDownstream)
        }

        // Auto populate test model input if empty and models fetched
        if (binding.etTestModelId.text.isNullOrBlank() && state.lastFetchedModels.isNotEmpty()) {
            binding.etTestModelId.setText(state.lastFetchedModels.first())
        }
        isInternalTextUpdating = false

        // 3. Downstream Key Restart Hint
        binding.tvKeyRestartHint.visibility = if (state.keyNeedsRestart) View.VISIBLE else View.GONE

        // 4. App Link
        binding.tvAppLink.text = state.appLink

        // 5. Gateway Start / Stop Button & State
        when (state.gatewayState) {
            GatewayState.IDLE -> {
                binding.btnStartStop.isEnabled = true
                binding.btnStartStop.text = getString(R.string.start_gateway)
                binding.btnStartStop.setBackgroundColor(ContextCompat.getColor(this, R.color.primary))
                binding.btnFetchModels.isEnabled = false
                binding.btnTestMessage.isEnabled = false
            }
            GatewayState.STARTING -> {
                binding.btnStartStop.isEnabled = false
                binding.btnStartStop.text = "正在启动..."
                binding.btnFetchModels.isEnabled = false
                binding.btnTestMessage.isEnabled = false
            }
            GatewayState.RUNNING -> {
                binding.btnStartStop.isEnabled = true
                binding.btnStartStop.text = getString(R.string.stop_gateway)
                binding.btnStartStop.setBackgroundColor(ContextCompat.getColor(this, R.color.error))
                binding.btnFetchModels.isEnabled = !state.isTesting
                binding.btnTestMessage.isEnabled = !state.isTesting
            }
            GatewayState.STOPPING -> {
                binding.btnStartStop.isEnabled = false
                binding.btnStartStop.text = "正在停止..."
                binding.btnFetchModels.isEnabled = false
                binding.btnTestMessage.isEnabled = false
            }
            GatewayState.ERROR -> {
                binding.btnStartStop.isEnabled = true
                binding.btnStartStop.text = "重试启动"
                binding.btnStartStop.setBackgroundColor(ContextCompat.getColor(this, R.color.primary))
                binding.btnFetchModels.isEnabled = false
                binding.btnTestMessage.isEnabled = false
            }
        }

        // 6. Test progress & result
        binding.pbTesting.visibility = if (state.isTesting) View.VISIBLE else View.GONE
        binding.tvTestResult.text = state.testResult

        // Status Toast if any
        if (state.statusMessage.isNotEmpty()) {
            Toast.makeText(this, state.statusMessage, Toast.LENGTH_SHORT).show()
            viewModel.clearStatusMessage()
        }
    }

    private fun showProviderDialog() {
        val state = viewModel.uiState.value
        if (state.gatewayState == GatewayState.RUNNING || state.gatewayState == GatewayState.STARTING) {
            Toast.makeText(this, "网关运行中，切换供应商需先停止", Toast.LENGTH_SHORT).show()
            return
        }

        val providers = state.config.providers
        val currentId = state.config.currentProviderId

        val container = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(24, 16, 24, 16)
        }

        var alertDialog: AlertDialog? = null

        for (p in providers) {
            val itemView = LayoutInflater.from(this).inflate(R.layout.dialog_provider_item, container, false)
            val rb = itemView.findViewById<RadioButton>(R.id.rbSelected)
            val tvName = itemView.findViewById<TextView>(R.id.tvProviderName)
            val tvHost = itemView.findViewById<TextView>(R.id.tvProviderUrlHost)
            val btnRename = itemView.findViewById<View>(R.id.btnRenameProvider)
            val btnDelete = itemView.findViewById<View>(R.id.btnDeleteProvider)

            rb.isChecked = (p.id == currentId)
            tvName.text = p.name

            btnRename.setOnClickListener {
                showRenameProviderDialog(p.id, p.name)
                alertDialog?.dismiss()
            }
            val host = try {
                if (p.upstreamUrl.isNotEmpty()) Uri.parse(p.upstreamUrl).host ?: p.upstreamUrl else "未配置 URL"
            } catch (_: Exception) {
                p.upstreamUrl
            }
            tvHost.text = host

            if (providers.size > 1) {
                btnDelete.visibility = View.VISIBLE
                btnDelete.setOnClickListener {
                    MaterialAlertDialogBuilder(this)
                        .setTitle("确认删除")
                        .setMessage("确定要删除供应商「${p.name}」吗？")
                        .setPositiveButton("删除") { _, _ ->
                            viewModel.deleteProvider(p.id)
                            alertDialog?.dismiss()
                        }
                        .setNegativeButton("取消", null)
                        .show()
                }
            } else {
                btnDelete.visibility = View.GONE
            }

            itemView.setOnClickListener {
                viewModel.selectProvider(p.id)
                alertDialog?.dismiss()
            }

            container.addView(itemView)
        }

        alertDialog = MaterialAlertDialogBuilder(this)
            .setTitle("选择供应商")
            .setView(container)
            .setPositiveButton("新建供应商") { _, _ ->
                showCreateProviderDialog()
            }
            .setNegativeButton("关闭", null)
            .create()

        alertDialog.show()
    }

    private fun showCreateProviderDialog() {
        val input = EditText(this).apply {
            hint = "供应商名称 (如: 供应商 ${viewModel.uiState.value.config.providers.size + 1})"
            setSingleLine()
        }
        val container = LinearLayout(this).apply {
            setPadding(48, 24, 48, 24)
            addView(input)
        }

        MaterialAlertDialogBuilder(this)
            .setTitle("新建供应商")
            .setView(container)
            .setPositiveButton("创建") { _, _ ->
                val name = input.text.toString().trim()
                viewModel.createProvider(name)
            }
            .setNegativeButton("取消", null)
            .show()
    }
    private fun showRenameProviderDialog(providerId: String, currentName: String) {
        val input = EditText(this).apply {
            setText(currentName)
            setSelection(currentName.length)
            setSingleLine()
        }
        val container = LinearLayout(this).apply {
            setPadding(48, 24, 48, 24)
            addView(input)
        }

        MaterialAlertDialogBuilder(this)
            .setTitle("重命名供应商")
            .setView(container)
            .setPositiveButton("保存") { _, _ ->
                val newName = input.text.toString().trim()
                if (newName.isNotEmpty()) {
                    viewModel.renameProvider(providerId, newName)
                }
            }
            .setNegativeButton("取消", null)
            .show()
    }

    private fun copyToClipboard(label: String, text: String) {
        val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val clip = ClipData.newPlainText(label, text)
        clipboard.setPrimaryClip(clip)
    }
}
