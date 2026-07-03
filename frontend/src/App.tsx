import { useState, useEffect, useRef } from 'react'
import './App.css'

interface Stock {
  ticker: string
  price: number
}

interface PriceUpdate {
  ticker: string
  price: number
}

interface SubscribeMessage {
  action: 'subscribe' | 'unsubscribe'
  ticker: string
}

const getApiBase = () => {
  const host = window.location.hostname
  const port = window.location.port || (window.location.protocol === 'https:' ? '443' : '80')
  const apiPort = port === '5173' ? '8080' : port
  return `${window.location.protocol}//${host}:${apiPort}/api`
}

function App() {
  const [watchlist, setWatchlist] = useState<Stock[]>([])
  const [tickerInput, setTickerInput] = useState('')
  const [connected, setConnected] = useState(false)
  const wsRef = useRef<WebSocket | null>(null)
  // Mirror watchlist in a ref so the WebSocket onopen handler (which closes over
  // state captured at connect time) always reads the current set of tickers.
  const watchlistRef = useRef<Stock[]>([])
  const subscribedRef = useRef<Set<string>>(new Set())

  useEffect(() => {
    watchlistRef.current = watchlist
  }, [watchlist])

  useEffect(() => {
    loadWatchlist()
    connectWebSocket()
    return () => {
      if (wsRef.current) {
        wsRef.current.close()
      }
    }
  }, [])

  // Subscribe to any watchlist tickers not yet subscribed whenever the
  // watchlist changes while the socket is open. This covers DB-loaded tickers
  // arriving after the socket connects, without requiring a re-add.
  useEffect(() => {
    if (!connected) return
    watchlist.forEach((stock) => {
      if (!subscribedRef.current.has(stock.ticker)) {
        subscribeToTicker(stock.ticker)
      }
    })
  }, [watchlist, connected])

  const loadWatchlist = async () => {
    try {
      const response = await fetch(`${getApiBase()}/watchlist`)
      const data = await response.json()
      setWatchlist(data)
    } catch (error) {
      console.error('Failed to load watchlist:', error)
    }
  }

  const connectWebSocket = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.hostname
    const port = window.location.port || (window.location.protocol === 'https:' ? '443' : '80')
    const wsUrl = `${protocol}//${host}:${port === '5173' ? '8080' : port}/ws`
    console.log('Attempting WebSocket connection to:', wsUrl)
    
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      console.log('WebSocket already connected')
      return
    }

    const websocket = new WebSocket(wsUrl)
    wsRef.current = websocket

    websocket.onopen = () => {
      // A fresh connection has no server-side subscriptions yet.
      subscribedRef.current.clear()
      setConnected(true)
      console.log('WebSocket connected successfully')
      // Resubscribe to all current watchlist tickers on connection/reconnection.
      // Read from the ref to avoid a stale closure over the initial (empty) state.
      watchlistRef.current.forEach((stock: Stock) => {
        subscribeToTicker(stock.ticker)
      })
    }

    websocket.onclose = (event) => {
      setConnected(false)
      wsRef.current = null
      subscribedRef.current.clear()
      console.log('WebSocket disconnected:', event.code, event.reason)
      // Attempt to reconnect after 3 seconds
      setTimeout(connectWebSocket, 3000)
    }

    websocket.onerror = (error) => {
      console.error('WebSocket error:', error)
    }

    websocket.onmessage = (event) => {
      const data: PriceUpdate = JSON.parse(event.data)
      updatePrice(data.ticker, data.price)
    }
  }

  const subscribeToTicker = (ticker: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const message: SubscribeMessage = { action: 'subscribe', ticker }
      wsRef.current.send(JSON.stringify(message))
      subscribedRef.current.add(ticker)
      console.log('Subscribed to ticker:', ticker)
    }
  }

  const unsubscribeFromTicker = (ticker: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const message: SubscribeMessage = { action: 'unsubscribe', ticker }
      wsRef.current.send(JSON.stringify(message))
      subscribedRef.current.delete(ticker)
      console.log('Unsubscribed from ticker:', ticker)
    }
  }

  const updatePrice = (ticker: string, price: number) => {
    setWatchlist(prev => 
      prev.map(stock => 
        stock.ticker === ticker ? { ...stock, price } : stock
      )
    )
  }

  const addStock = async () => {
    const ticker = tickerInput.trim().toUpperCase()
    if (!ticker) return

    try {
      const response = await fetch(`${getApiBase()}/watchlist`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ticker }),
      })

      if (response.ok) {
        const data = await response.json()
        setWatchlist(prev => [...prev, data])
        setTickerInput('')
        // Subscribe to the newly added ticker
        subscribeToTicker(ticker)
      } else if (response.status === 409) {
        alert('Ticker already in watchlist')
      } else {
        alert('Failed to add ticker')
      }
    } catch (error) {
      console.error('Failed to add stock:', error)
      alert('Failed to add stock')
    }
  }

  const removeStock = async (ticker: string) => {
    try {
      const response = await fetch(`${getApiBase()}/watchlist/${ticker}`, {
        method: 'DELETE',
      })

      if (response.ok) {
        setWatchlist(prev => prev.filter(stock => stock.ticker !== ticker))
        // Unsubscribe from the removed ticker
        unsubscribeFromTicker(ticker)
      }
    } catch (error) {
      console.error('Failed to remove stock:', error)
    }
  }

  return (
    <div className="container">
      <header>
        <h1>📈 Stock Ticker Watcher</h1>
        <p className="subtitle">Real-time price updates</p>
      </header>

      <div className="add-stock-form">
        <input
          type="text"
          value={tickerInput}
          onChange={(e) => setTickerInput(e.target.value)}
          placeholder="Enter ticker (e.g., AAPL)"
          maxLength={10}
          onKeyPress={(e) => e.key === 'Enter' && addStock()}
        />
        <button onClick={addStock}>Add to Watchlist</button>
      </div>

      <div className="watchlist">
        {watchlist.length === 0 ? (
          <div className="empty-state">
            <p>Your watchlist is empty</p>
            <p>Add some tickers to get started!</p>
          </div>
        ) : (
          watchlist.map((stock) => (
            <div key={stock.ticker} className="stock-item">
              <div className="stock-info">
                <span className="ticker">{stock.ticker}</span>
                <span className="price">${stock.price.toFixed(2)}</span>
              </div>
              <button 
                className="remove-btn"
                onClick={() => removeStock(stock.ticker)}
              >
                Remove
              </button>
            </div>
          ))
        )}
      </div>

      <div className="status">
        <span className={`status-indicator ${connected ? 'connected' : 'disconnected'}`} />
        <span className="status-text">{connected ? 'Connected' : 'Disconnected'}</span>
      </div>
    </div>
  )
}

export default App
