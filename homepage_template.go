package main

// homepageTemplate is the HTML template for the home page
const homepageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>IPFS Blossomnator Tabajara</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            padding: 20px;
            color: #333;
        }
        .container {
            max-width: 900px;
            margin: 0 auto;
            background: white;
            border-radius: 12px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 40px;
            text-align: center;
        }
        .header h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        .header p {
            font-size: 1.1em;
            opacity: 0.9;
        }
        .content {
            padding: 40px;
        }
        .section {
            margin-bottom: 40px;
        }
        .section h2 {
            color: #667eea;
            font-size: 1.8em;
            margin-bottom: 20px;
            padding-bottom: 10px;
            border-bottom: 2px solid #e0e0e0;
        }
        .section h3 {
            color: #764ba2;
            font-size: 1.3em;
            margin-top: 20px;
            margin-bottom: 10px;
        }
        .status-badge {
            display: inline-block;
            padding: 8px 16px;
            border-radius: 20px;
            font-weight: bold;
            font-size: 0.9em;
            margin-left: 10px;
        }
        .status-healthy {
            background: #4caf50;
            color: white;
        }
        .status-unhealthy {
            background: #f44336;
            color: white;
        }
        .health-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        .health-card {
            background: #f5f5f5;
            padding: 20px;
            border-radius: 8px;
            border-left: 4px solid #667eea;
        }
        .health-card h4 {
            color: #667eea;
            margin-bottom: 10px;
            font-size: 1.1em;
        }
        .health-card .value {
            font-size: 1.5em;
            font-weight: bold;
            color: #333;
        }
        .health-card .label {
            color: #666;
            font-size: 0.9em;
            margin-top: 5px;
        }
        .code-block {
            background: #2d2d2d;
            color: #f8f8f2;
            padding: 20px;
            border-radius: 8px;
            overflow-x: auto;
            font-family: 'Courier New', monospace;
            font-size: 0.9em;
            line-height: 1.6;
            margin: 15px 0;
        }
        .code-block code {
            color: #f8f8f2;
        }
        .endpoint {
            background: #f9f9f9;
            padding: 15px;
            border-radius: 8px;
            margin: 10px 0;
            border-left: 3px solid #667eea;
        }
        .endpoint strong {
            color: #667eea;
        }
        ul {
            margin-left: 20px;
            margin-top: 10px;
        }
        li {
            margin: 8px 0;
            line-height: 1.6;
        }
        .footer {
            text-align: center;
            padding: 20px;
            background: #f5f5f5;
            color: #666;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🌺 IPFS Blossomnator Tabajara</h1>
            <p>Nostr Blossom Server with IPFS Backend</p>
            <span class="status-badge status-%s">%s</span>
        </div>
        <div class="content">
            <div class="section">
                <h2>📊 Health Status</h2>
                <div class="health-grid">
                    <div class="health-card">
                        <h4>Database</h4>
                        <div class="value">%s</div>
                        <div class="label">%s</div>
                    </div>
                    <div class="health-card">
                        <h4>IPFS</h4>
                        <div class="value">%s</div>
                        <div class="label">%s</div>
                    </div>
                    <div class="health-card">
                        <h4>Memory</h4>
                        <div class="value">%d MB</div>
                        <div class="label">%s (max: %d MB)</div>
                    </div>
                    <div class="health-card">
                        <h4>Goroutines</h4>
                        <div class="value">%d</div>
                        <div class="label">%s (max: %d)</div>
                    </div>
                </div>
            </div>

            <div class="section">
                <h2>📖 Usage</h2>
                <h3>Configure in Nostr Clients</h3>
                <p>To use this Blossom server, configure your Nostr client to use this server address for media uploads:</p>
                <div class="code-block">
<code>%s</code>
                </div>
                <p>Most Nostr clients allow you to configure a custom Blossom server in their settings. Look for:</p>
                <ul>
                    <li><strong>Media/Blob Storage Settings</strong> - Set the Blossom server URL</li>
                    <li><strong>File Upload Configuration</strong> - Specify this server for media uploads</li>
                    <li><strong>Blossom Server URL</strong> - Enter the server address above</li>
                </ul>
                <p>When you upload files through your Nostr client:</p>
                <ul>
                    <li>Files are automatically stored in IPFS</li>
                    <li>The server returns IPFS gateway URLs in responses</li>
                    <li>SHA256 hashes are mapped to IPFS CIDs</li>
                    <li>Files are accessible via both Blossom URLs and IPFS gateway URLs</li>
                </ul>

                <h3>Access Files</h3>
                <p>Uploaded files can be accessed via:</p>
                <ul>
                    <li><strong>Blossom URL:</strong> <code>%s/&lt;sha256&gt;.&lt;ext&gt;</code> (redirects to IPFS gateway)</li>
                    <li><strong>IPFS Gateway:</strong> <code>%s&lt;cid&gt;?filename=file.&lt;ext&gt;</code></li>
                </ul>
            </div>

            <div class="section">
                <h2>🔗 API Endpoints</h2>
                <div class="endpoint">
                    <strong>GET /</strong> - This home page
                </div>
                <div class="endpoint">
                    <strong>GET /health</strong> - Health check endpoint (returns JSON)
                </div>
                <div class="endpoint">
                    <strong>POST /blossom</strong> - Upload a file (Blossom protocol)
                </div>
                <div class="endpoint">
                    <strong>GET /list/&lt;pubkey&gt;</strong> - List files for a pubkey (Blossom protocol)
                </div>
                <div class="endpoint">
                    <strong>GET /&lt;sha256&gt;.&lt;ext&gt;</strong> - Get file (redirects to IPFS gateway)
                </div>
            </div>
        </div>
        <div class="footer">
            <p>IPFS Gateway: %s | Server: %s</p>
        </div>
    </div>
</body>
</html>`
