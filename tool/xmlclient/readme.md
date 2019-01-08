# Tools

## Intro

测试 go supervisord 工具集

## Usage

+ 1 启动 virtualenv

    > pip install virtualenv && virtualenv venv && . venv/bin/activate

+ 2 修改 config.py 中本地监听地址 Localhost 和 ListenPort

    > 如果当前环境是 development，则修改 DevConfig；如果是 test，则修改 TestConfig；如果是 production，则修改 ProdConfig。
