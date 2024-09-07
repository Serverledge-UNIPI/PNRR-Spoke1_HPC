def solve_n_queens(n):
    def is_safe(board, row, col):
        for i in range(col):
            if board[row][i] == 1:
                return False
        for i, j in zip(range(row, -1, -1), range(col, -1, -1)):
            if board[i][j] == 1:
                return False
        for i, j in zip(range(row, n, 1), range(col, -1, -1)):
            if board[i][j] == 1:
                return False
        return True

    def solve_recursively(board, col):
        if col >= n:
            solutions.append(["".join("Q" if cell == 1 else "." for cell in row) for row in board])
            return

        for i in range(n):
            if is_safe(board, i, col):
                board[i][col] = 1
                solve_recursively(board, col + 1)
                board[i][col] = 0

    solutions = []
    board = [[0] * n for _ in range(n)]
    solve_recursively(board, 0)
    return solutions

def handler(params, context):
    try:
        n = int(params["n"])
        solutions = solve_n_queens(n)
        return {"Solutions": solutions}
    except:
        return {}