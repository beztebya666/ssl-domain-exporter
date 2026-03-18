import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { error: Error | null }

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex items-center justify-center h-screen bg-slate-950">
          <div className="max-w-lg p-6 bg-red-900/20 border border-red-700 rounded-xl text-center">
            <h2 className="text-red-400 font-bold text-lg mb-2">Runtime Error</h2>
            <pre className="text-red-300 text-xs text-left bg-red-900/20 p-3 rounded overflow-auto">
              {this.state.error.message}
              {'\n'}
              {this.state.error.stack}
            </pre>
            <button
              className="mt-4 btn-primary"
              onClick={() => this.setState({ error: null })}
            >
              Retry
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
